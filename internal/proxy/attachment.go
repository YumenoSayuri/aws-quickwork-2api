package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// attachmentClient is shared by uploads; the request context carries the
// deadline.
var attachmentClient = &http.Client{
	Timeout: 3 * time.Minute,
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
	return "objects"
}

// attachmentObjects builds the object form used by most candidate shapes.
func attachmentObjects(ids []string) []map[string]any {
	list := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		list = append(list, map[string]any{"attachmentId": id})
	}
	return list
}

// attachPayload writes the attachment references into a user_message payload.
func attachPayload(payload map[string]any, ids []string, shapeOverride string) {
	if len(ids) == 0 {
		return
	}
	objs := func() []map[string]any { return attachmentObjects(ids) }
	plain := func() []string { return append([]string(nil), ids...) }

	switch {
	case attachmentShape(shapeOverride) == "ids":
		payload["attachments"] = plain()
	case attachmentShape(shapeOverride) == "ids-underscore":
		payload["attachment_ids"] = plain()
	case attachmentShape(shapeOverride) == "camel-ids":
		payload["attachmentIds"] = plain()
	case attachmentShape(shapeOverride) == "camel-objects":
		payload["Attachments"] = objs()
	case attachmentShape(shapeOverride) == "files":
		payload["files"] = objs()
	case attachmentShape(shapeOverride) == "file-ids":
		payload["fileIds"] = plain()
	case attachmentShape(shapeOverride) == "uploaded-files":
		payload["uploadedFiles"] = objs()
	case attachmentShape(shapeOverride) == "message-attachments":
		payload["message_attachments"] = objs()
	case strings.HasPrefix(attachmentShape(shapeOverride), "key:"):
		key := strings.TrimPrefix(attachmentShape(shapeOverride), "key:")
		list := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			list = append(list, map[string]any{key: id})
		}
		payload["attachments"] = list
	default:
		payload["attachments"] = objs()
	}
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
	req.Header.Set("Content-Type", contentType)

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
	return out.AttachmentID, nil
}
