package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxAttachmentBytes caps one uploaded file.
const maxAttachmentBytes = 20 * 1024 * 1024

// handleFiles implements a small OpenAI-compatible file upload:
//
//	POST /v1/files      (multipart/form-data: file=<bytes>, purpose=optional)
//	-> {"id":"file-<attachmentId>","object":"file","bytes":N,"filename":"..."}
//
// The returned id can be passed in a chat request as an image_url part
// ("file://<id>") or through the X-Quickwork-Attachments header.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	if !s.tokens.Allowed(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "invalid or missing API key")
		return
	}
	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "expected multipart/form-data with a file field")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing file field")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, maxAttachmentBytes+1))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "cannot read uploaded file")
		return
	}
	if len(body) > maxAttachmentBytes {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large",
			fmt.Sprintf("file exceeds %d bytes", maxAttachmentBytes))
		return
	}

	name := header.Filename
	contentType := header.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" || contentType == "application/octet-stream" {
		contentType = contentTypeByName(name)
	}

	ctx := r.Context()
	acct, err := s.accounts.Next(ctx)
	if err != nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "no_accounts", err.Error())
		return
	}
	sess := SessionFromAccount(acct, s.cfg)
	if err := sess.validate(); err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	id, err := s.upstream.UploadAttachment(ctx, sess, name, contentType, body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upload_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         "file-" + id,
		"object":     "file",
		"bytes":      len(body),
		"filename":   name,
		"purpose":    r.FormValue("purpose"),
		"created_at": time.Now().Unix(),
	})
}

// contentTypeByName guesses a MIME type from the file extension.
func contentTypeByName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".md"):
		return "text/markdown"
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "text/html"
	case strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".log"):
		return "text/plain"
	}
	return "application/octet-stream"
}

// attachmentIDsFromRequest collects attachment ids from the request headers and
// from any "file://" (or bare "file-") image parts in the messages.
func attachmentIDsFromRequest(r *http.Request, msgs []Message) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		id = strings.TrimPrefix(id, "file://")
		id = strings.TrimPrefix(id, "file-")
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, raw := range strings.Split(r.Header.Get("X-Quickwork-Attachments"), ",") {
		add(raw)
	}
	for _, m := range msgs {
		parts, ok := m.Content.([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := p["type"].(string)
			if typ == "file" {
				if f, ok := p["file"].(map[string]any); ok {
					if v, ok := f["file_id"].(string); ok {
						add(v)
					}
				}
			}
			if typ == "image_url" {
				u := imagePartURL(p)
				if strings.HasPrefix(u, "file://") || strings.HasPrefix(u, "file-") {
					add(u)
				}
			}
		}
	}
	return out
}
