package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// attachmentClient is shared by uploads; the request context carries the
// deadline.
var attachmentClient = &http.Client{
	Timeout: 3 * time.Minute,
}

// attachmentMeta is what the upstream wire format needs besides the id.
type attachmentMeta struct {
	Name       string
	Size       int
	MimeType   string
	SHA256     string
	Type       string // "file" or "image"
	DataURL    string // optional inline payload (small files only)
	ServerPath string
}

// attachmentRegistry remembers uploads made through /v1/files so a later chat
// request can reference them by id alone.
var attachmentRegistry sync.Map

func rememberAttachment(id string, meta attachmentMeta) {
	attachmentRegistry.Store(id, meta)
}

func lookupAttachment(id string) (attachmentMeta, bool) {
	if v, ok := attachmentRegistry.Load(id); ok {
		if meta, ok := v.(attachmentMeta); ok {
			return meta, true
		}
	}
	return attachmentMeta{}, false
}

// attachmentShape selects how uploaded attachments are referenced inside the
// user_message payload. The upstream schema is not documented, so the shape is
// overridable per request (header) or per process (env).
func attachmentShape(override string) string {
	if v := strings.TrimSpace(override); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("QUICKWORK_ATTACHMENT_SHAPE")); v != "" {
		return v
	}
	// Inline delivery is the only shape verified against the live service: the
	// file rides as a data URL and the backend ingests it directly. The
	// out-of-band form needs the client-side delivery step, which is still
	// unimplemented.
	return "inline"
}

// wireAttachment mirrors the client's serializer:
//
//	{type, name, size, mimeType, path, attachmentId, contentSha256?, data?}
//
// data is only present when the attachment was NOT bound out of band.
func wireAttachment(id, shape string) map[string]any {
	meta, _ := lookupAttachment(id)
	typ := meta.Type
	if typ == "" {
		if strings.HasPrefix(meta.MimeType, "image/") {
			typ = "image"
		} else {
			typ = "file"
		}
	}
	name := meta.Name
	if name == "" {
		name = id
	}
	item := map[string]any{
		"type":         typ,
		"name":         name,
		"size":         meta.Size,
		"mimeType":     meta.MimeType,
		"path":         "attached_files/" + name,
		"attachmentId": id,
	}
	if meta.SHA256 != "" {
		item["contentSha256"] = meta.SHA256
	}
	if meta.ServerPath != "" {
		item["path"] = meta.ServerPath
	}

	switch shape {
	case "id-only":
		return map[string]any{"attachmentId": id}
	case "no-path":
		delete(item, "path")
	case "no-sha":
		delete(item, "contentSha256")
	case "inline":
		// Inline delivery: the whole file travels as a data URL and the backend
		// ingests it directly (no out-of-band binding).
		if meta.DataURL != "" {
			inline := map[string]any{
				"type":     typ,
				"name":     name,
				"size":     meta.Size,
				"mimeType": meta.MimeType,
				"data":     meta.DataURL,
			}
			return inline
		}
	}
	return item
}

// attachmentBytes reports how many bytes the attachment references add to the
// frame once serialized. Inline data URLs dominate; plain references are tiny.
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

func dataLen(item map[string]any) int {
	if s, ok := item["data"].(string); ok {
		return len(s)
	}
	return 0
}

// trimInlineAttachments sheds inline `data` payloads, heaviest first, until the
// allowance is met. The reference (id/name) stays so the file is still visible.
func trimInlineAttachments(items []map[string]any, allowance int) []map[string]any {
	if len(items) == 0 {
		return items
	}
	total := attachmentBytes(items)
	if total <= allowance {
		return items
	}
	order := make([]int, 0, len(items))
	for i, item := range items {
		if _, ok := item["data"]; ok {
			order = append(order, i)
		}
	}
	// Insertion sort, heaviest first (lists are tiny).
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && dataLen(items[order[j]]) > dataLen(items[order[j-1]]); j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
	for _, idx := range order {
		if total <= allowance {
			break
		}
		item := items[idx]
		total -= dataLen(item)
		delete(item, "data")
		item["note"] = "inline payload omitted: file too large for one frame"
	}
	return items
}

// buildAttachmentRefs turns ids into the wire descriptors the frame carries.
// The caller trims these against the frame budget before sending.
func buildAttachmentRefs(ids []string, shapeOverride string) []map[string]any {
	if len(ids) == 0 {
		return nil
	}
	shape := attachmentShape(shapeOverride)
	list := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		list = append(list, wireAttachment(id, shape))
	}
	return list
}

// attachPayload writes the prepared attachment descriptors into a
// user_message payload.
func attachPayload(payload map[string]any, items []map[string]any) {
	if len(items) == 0 {
		return
	}
	payload["attachments"] = items
}

// dataURL builds an RFC 2397 data URL for inline attachment delivery.
func dataURL(mimeType string, body []byte) string {
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(body)
}

// UploadAttachment pushes raw bytes to the account's data plane and returns the
// attachment id the conversation service expects.
//
// The endpoint is a query-parameter + raw-body call, not JSON:
//
//	POST {tenant}/integration/quick-work/upload-attachment
//	     ?name=report.md&contentLength=1234&contentType=text/markdown
//	body: <raw file bytes>
//	-> {"attachmentId":"..."}
func (u *Upstream) UploadAttachment(ctx context.Context, sess *Session, name, contentType string, body []byte) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(sess.TenantURL), "/")
	if base == "" {
		return "", fmt.Errorf("account has no tenant url")
	}
	if strings.TrimSpace(name) == "" {
		name = "attachment"
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("contentLength", fmt.Sprintf("%d", len(body)))
	q.Set("contentType", contentType)
	endpoint := base + "/integration/quick-work/upload-attachment?" + q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sess.jwt())
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := attachmentClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload attachment: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload attachment: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		AttachmentID string `json:"attachmentId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("upload attachment: unexpected response %s", strings.TrimSpace(string(raw)))
	}
	if out.AttachmentID == "" {
		return "", fmt.Errorf("upload attachment: no attachmentId in %s", strings.TrimSpace(string(raw)))
	}

	sum := sha256.Sum256(body)
	meta := attachmentMeta{
		Name:     name,
		Size:     len(body),
		MimeType: contentType,
		SHA256:   hex.EncodeToString(sum[:]),
	}
	// Small payloads can ride inline as a data URL, which needs no delivery step.
	if len(body) <= inlineLimit() {
		meta.DataURL = dataURL(contentType, body)
	}
	rememberAttachment(out.AttachmentID, meta)
	return out.AttachmentID, nil
}

// defaultInlineLimit is the largest body worth carrying inside the frame. The
// upstream frame budget is ~62 KB, so keep well under it by default.
const defaultInlineLimit = 40 * 1024

// inlineLimit reports the inline attachment cap, overridable for experiments.
func inlineLimit() int {
	if v := strings.TrimSpace(os.Getenv("QUICKWORK_INLINE_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultInlineLimit
}

// attachBypassBudget lets an oversized inline payload through untouched, which
// is how the upstream frame limit is measured.
func attachBypassBudget() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("QUICKWORK_ATTACH_BYPASS_BUDGET")))
	return v == "1" || v == "true" || v == "yes"
}

// DescribeAttachmentDLP reports the data-loss-prevention verdict for an uploaded
// attachment:
//
//	GET {tenant}/integration/quick-work/attachment-dlp-status?attachmentId=
//	-> {"dlpJobStatus":"...","policyAction":"..."}
func (u *Upstream) DescribeAttachmentDLP(ctx context.Context, sess *Session, attachmentID string) (string, string, error) {
	base := strings.TrimRight(strings.TrimSpace(sess.TenantURL), "/")
	if base == "" {
		return "", "", fmt.Errorf("account has no tenant url")
	}
	q := url.Values{}
	q.Set("attachmentId", attachmentID)
	endpoint := base + "/integration/quick-work/attachment-dlp-status?" + q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+sess.jwt())

	resp, err := attachmentClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("dlp status: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("dlp status: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Status string `json:"dlpJobStatus"`
		Policy string `json:"policyAction"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Status, out.Policy, nil
}
