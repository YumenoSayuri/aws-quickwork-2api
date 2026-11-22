package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxAttachmentBytes caps one uploaded file.
const maxAttachmentBytes = 25 * 1024 * 1024

// handleFiles implements a small OpenAI-compatible file upload:
//
//	POST /v1/files   (multipart/form-data: file=<bytes>)
//	-> {"id":"file-<uuid>","object":"file","bytes":N,"filename":"..."}
//
// The bytes are parked here, not uploaded yet: the real upload happens inside
// the chat turn so the attachment and the conversation share one account.
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
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error",
			"expected multipart/form-data with a file field")
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
	id := storeAttachment(StoredAttachment{
		Name:      name,
		MimeType:  contentType,
		Kind:      attachmentKind(contentType),
		Data:      body,
		CreatedAt: time.Now(),
	})

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

// attachmentsForTurn collects everything the turn should carry: files parked by
// /v1/files plus image parts embedded directly in the messages.
func attachmentsForTurn(r *http.Request, msgs []Message) []RawAttachment {
	var out []RawAttachment
	seen := map[string]bool{}

	for _, raw := range strings.Split(r.Header.Get("X-Quickwork-Attachments"), ",") {
		id := strings.TrimSpace(raw)
		id = strings.TrimPrefix(id, "file://")
		id = strings.TrimPrefix(id, "file-")
		if id == "" || seen[id] {
			continue
		}
		st, ok := claimAttachment(id)
		if !ok {
			continue
		}
		seen[id] = true
		out = append(out, RawAttachment{
			Kind:     st.Kind,
			Name:     st.Name,
			MimeType: st.MimeType,
			Data:     st.Data,
		})
	}

	imgIdx := 0
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
			switch typ {
			case "image_url", "input_image":
				u := imagePartURL(p)
				if u == "" {
					continue
				}
				mt, data, ok := decodeDataURI(u)
				if !ok || len(data) == 0 || len(data) > maxAttachmentBytes {
					continue
				}
				imgIdx++
				if !strings.HasPrefix(strings.ToLower(mt), "image/") {
					mt = http.DetectContentType(data)
				}
				out = append(out, RawAttachment{
					Kind:     "image",
					Name:     defaultAttachmentName("image", imgIdx, mt),
					MimeType: mt,
					Data:     data,
				})
			case "file":
				f, ok := p["file"].(map[string]any)
				if !ok {
					continue
				}
				rawData, _ := f["file_data"].(string)
				if rawData == "" {
					continue
				}
				name, _ := f["filename"].(string)
				mt, data, ok := decodeDataURI(rawData)
				if !ok || len(data) == 0 || len(data) > maxAttachmentBytes {
					continue
				}
				out = append(out, RawAttachment{
					Kind:     attachmentKind(mt),
					Name:     name,
					MimeType: mt,
					Data:     data,
				})
			}
		}
	}
	return out
}
