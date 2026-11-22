package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// attachmentClient carries uploads, DLP polls and deliveries.
var attachmentClient = &http.Client{Timeout: 3 * time.Minute}

// maxInlineFallback is the largest body worth carrying inside the frame when
// the out-of-band path is unavailable. Base64 inflates by 1.33x and the frame
// budget is shared with the prompt, so this stays well under the hard limit.
const maxInlineFallback = 40 * 1024

// inlineLimit reports the inline fallback ceiling, overridable for experiments.
func inlineLimit() int {
	if v := strings.TrimSpace(os.Getenv("QUICKWORK_INLINE_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return maxInlineFallback
}

// attachBypassBudget lets an oversized inline payload through untouched, which
// is how the upstream frame limit is measured.
func attachBypassBudget() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("QUICKWORK_ATTACH_BYPASS_BUDGET")))
	return v == "1" || v == "true" || v == "yes"
}

// tenantBaseURL resolves the data plane host for an account.
func tenantBaseURL(sess *Session) string {
	if t := strings.TrimRight(strings.TrimSpace(sess.TenantURL), "/"); t != "" {
		return t
	}
	host := strings.TrimSpace(sess.WSHost)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "ws://")
	host = strings.TrimPrefix(host, "wss://")
	host = strings.TrimRight(host, "/")
	if host == "" {
		return ""
	}
	return "https://" + host
}

// uploadAttachmentBytes performs Step 1: raw bytes in, attachmentId out.
func (u *Upstream) uploadAttachmentBytes(ctx context.Context, sess *Session, name, mimeType string, data []byte) (string, error) {
	base := tenantBaseURL(sess)
	if base == "" {
		return "", fmt.Errorf("account has no tenant url")
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("contentType", mimeType)
	q.Set("contentLength", fmt.Sprintf("%d", len(data)))
	endpoint := base + "/integration/quick-work/upload-attachment?" + q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sess.jwt())
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-amzn-qs-internal-trace-id", newUUID())

	resp, err := attachmentClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload-attachment: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload-attachment: HTTP %d %s", resp.StatusCode, trimForLog(string(raw)))
	}
	var out struct {
		AttachmentID string `json:"attachmentId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.AttachmentID == "" {
		return "", fmt.Errorf("upload-attachment: no attachmentId in %s", trimForLog(string(raw)))
	}
	return out.AttachmentID, nil
}

// waitAttachmentDLP performs Step 2: poll the scan verdict. A 404/405 means the
// environment has DLP switched off, treated as a pass.
func (u *Upstream) waitAttachmentDLP(ctx context.Context, sess *Session, attachmentID string) error {
	base := tenantBaseURL(sess)
	if base == "" {
		return fmt.Errorf("account has no tenant url")
	}
	endpoint := base + "/integration/quick-work/attachment-dlp-status?" +
		url.Values{"attachmentId": []string{attachmentID}}.Encode()

	for attempt := 0; attempt < 20; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+sess.jwt())
		req.Header.Set("x-amzn-qs-internal-trace-id", newUUID())

		resp, err := attachmentClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		status := resp.StatusCode
		resp.Body.Close()

		if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
			return nil
		}
		if status >= 200 && status < 300 {
			var res struct {
				DlpJobStatus string `json:"dlpJobStatus"`
				Status       string `json:"status"`
				PolicyAction string `json:"policyAction"`
				Action       string `json:"action"`
			}
			if err := json.Unmarshal(raw, &res); err == nil {
				st := strings.ToUpper(strings.TrimSpace(firstNonEmpty(res.DlpJobStatus, res.Status)))
				act := strings.ToUpper(strings.TrimSpace(firstNonEmpty(res.PolicyAction, res.Action)))
				switch st {
				case "NOT_APPLICABLE", "SKIPPED":
					return nil
				case "COMPLETED":
					if act == "BLOCK" || act == "DENY" {
						return fmt.Errorf("attachment blocked by data loss prevention (%s)", act)
					}
					return nil
				case "FAILED", "ERROR":
					return fmt.Errorf("attachment DLP scan failed (%s)", st)
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

// DeliverAttachment performs Step 3: bind the upload into a session workspace
// through the invokeAgent envelope on the data plane.
func (u *Upstream) DeliverAttachment(ctx context.Context, sess *Session, sessionID, attachmentID string) (string, error) {
	base := tenantBaseURL(sess)
	if base == "" {
		return "", fmt.Errorf("account has no tenant url")
	}
	invoke := map[string]any{
		"path":        "files/upload/" + url.PathEscape(sessionID),
		"queryParams": map[string]string{"_method": "POST"},
		"body":        map[string]any{"attachmentId": attachmentID},
	}
	if sess.WorkingSessionID != "" {
		invoke["workingSessionId"] = sess.WorkingSessionID
	}
	if sess.OutpostID != "" {
		invoke["outpostId"] = sess.OutpostID
	}
	envelope := map[string]any{
		"eventType": "invokeAgent",
		"eventBody": map[string]any{"invokeAgent": invoke},
	}
	bodyBytes, _ := json.Marshal(envelope)

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/integration/quick-work/qbs-event", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sess.jwt())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-amzn-qs-internal-trace-id", newUUID())

	resp, err := attachmentClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deliver attachment: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deliver attachment: HTTP %d %s", resp.StatusCode, trimForLog(string(raw)))
	}
	path, err := unwrapDeliverPath(raw)
	if err != nil {
		return "", fmt.Errorf("deliver attachment: %w (raw=%s)", err, trimForLog(string(raw)))
	}
	return path, nil
}

// unwrapDeliverPath digs the workspace path out of the doubly-nested response:
// eventResponse.invokeAgentResponse is a JSON string holding {status, body},
// and body is itself a JSON string holding {path} or {files:[{path}]}.
func unwrapDeliverPath(raw []byte) (string, error) {
	pick := func(b []byte) string {
		var v struct {
			Path  string `json:"path"`
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return ""
		}
		if v.Path != "" {
			return v.Path
		}
		if len(v.Files) > 0 {
			return v.Files[0].Path
		}
		return ""
	}
	if p := pick(raw); p != "" {
		return p, nil
	}

	var outer struct {
		ErrorMessage  string `json:"errorMessage"`
		EventResponse struct {
			InvokeAgentResponse json.RawMessage `json:"invokeAgentResponse"`
		} `json:"eventResponse"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return "", fmt.Errorf("unexpected response")
	}
	if strings.TrimSpace(outer.ErrorMessage) != "" {
		return "", fmt.Errorf("%s", outer.ErrorMessage)
	}
	inner := outer.EventResponse.InvokeAgentResponse
	if len(inner) == 0 {
		return "", fmt.Errorf("missing invokeAgentResponse")
	}
	var asString string
	if err := json.Unmarshal(inner, &asString); err == nil {
		inner = []byte(asString)
	}
	var api struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(inner, &api); err != nil {
		return "", fmt.Errorf("unexpected invokeAgentResponse")
	}
	if api.Status >= 400 {
		return "", fmt.Errorf("agent file delivery status %d: %s", api.Status, trimForLog(string(api.Body)))
	}
	var bodyString string
	if err := json.Unmarshal(api.Body, &bodyString); err == nil {
		api.Body = []byte(bodyString)
	}
	if p := pick(api.Body); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("delivery payload missing path")
}

// prepareAttachment runs the full out-of-band flow for one file and returns the
// descriptor the frame carries.
func (u *Upstream) prepareAttachment(ctx context.Context, sess *Session, threadID string, att RawAttachment, index int) (map[string]any, error) {
	name := sanitizeAttachmentName(att.Name)
	if name == "" {
		name = defaultAttachmentName(att.Kind, index, att.MimeType)
	}
	kind := att.Kind
	if kind == "" {
		kind = attachmentKind(att.MimeType)
	}
	descriptor := map[string]any{
		"type":          kind,
		"name":          name,
		"size":          len(att.Data),
		"mimeType":      att.MimeType,
		"contentSha256": sha256Hex(att.Data),
	}

	sha := sha256Hex(att.Data)
	var id string
	if cached, ok := lookupUpload(sess.AccountID, sha); ok && cached.MimeType == att.MimeType {
		// Same bytes already uploaded for this account: skip the transfer and the
		// DLP scan, only re-bind into this session.
		id = cached.AttachmentID
	} else {
		var err error
		id, err = u.uploadAttachmentBytes(ctx, sess, name, att.MimeType, att.Data)
		if err != nil {
			return u.inlineFallback(att, descriptor, name, kind, err)
		}
		if err := u.waitAttachmentDLP(ctx, sess, id); err != nil {
			return u.inlineFallback(att, descriptor, name, kind, err)
		}
		rememberUpload(sess.AccountID, sha, cachedUpload{
			AttachmentID: id,
			MimeType:     att.MimeType,
			Size:         len(att.Data),
			Name:         name,
		})
	}
	descriptor["attachmentId"] = id
	path, err := u.DeliverAttachment(ctx, sess, threadID, id)
	if err != nil {
		return u.inlineFallback(att, descriptor, name, kind, err)
	}
	descriptor["path"] = path
	delete(descriptor, "data")
	log.Printf("attachment %q uploaded as-is: %d bytes original, workspace=%s", name, len(att.Data), path)
	return descriptor, nil
}

// inlineFallback carries the bytes inside the frame when the out-of-band flow
// cannot be used. Only small files qualify.
func (u *Upstream) inlineFallback(att RawAttachment, descriptor map[string]any, name, kind string, cause error) (map[string]any, error) {
	if len(att.Data) > inlineLimit() {
		return nil, fmt.Errorf("attachment %q could not be delivered (%v) and is too large to inline (%d bytes)",
			name, cause, len(att.Data))
	}
	descriptor["type"] = kind
	descriptor["name"] = name
	descriptor["data"] = "data:" + att.MimeType + ";base64," + base64.StdEncoding.EncodeToString(att.Data)
	return descriptor, nil
}

// attachmentBytes reports how many bytes the descriptors add to the frame.
func attachmentBytes(items []map[string]any) int {
	if len(items) == 0 {
		return 0
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return 0
	}
	return len(raw)
}

func trimForLog(s string) string {
	if len(s) <= 240 {
		return s
	}
	return s[:240] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// newUUID returns a random RFC 4122 v4 identifier.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// decodeDataURI parses a data: URL into MIME type and bytes.
func decodeDataURI(raw string) (string, []byte, bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", nil, false
	}
	comma := strings.IndexByte(raw, ',')
	if comma <= 5 {
		return "", nil, false
	}
	header := raw[5:comma]
	payload := raw[comma+1:]
	mimeType := "application/octet-stream"
	base64Encoded := false
	for i, part := range strings.Split(header, ";") {
		p := strings.TrimSpace(part)
		if i == 0 && p != "" && !strings.EqualFold(p, "base64") {
			mimeType = p
		} else if strings.EqualFold(p, "base64") {
			base64Encoded = true
		}
	}
	if !base64Encoded {
		unescaped, err := url.QueryUnescape(payload)
		if err != nil {
			return "", nil, false
		}
		return mimeType, []byte(unescaped), true
	}
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', ' ', '\t':
			return -1
		}
		return r
	}, payload)
	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(clean)
		if err != nil {
			return "", nil, false
		}
	}
	return mimeType, decoded, true
}
