package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RawAttachment is one file bound for the upstream conversation.
type RawAttachment struct {
	Kind     string // "image" or "file"
	Name     string
	MimeType string
	Data     []byte
}

// StoredAttachment is a file parked by /v1/files until a chat turn claims it.
// Upload happens inside the turn so the attachment and the conversation share
// one account — otherwise the workspace lookup fails with "Attachment not found".
type StoredAttachment struct {
	Name      string
	MimeType  string
	Kind      string
	Data      []byte
	CreatedAt time.Time
}

var (
	attachmentStore   sync.Map // id -> StoredAttachment
	attachmentStoreMu sync.Mutex
)

// defaultStoredBudget bounds how many bytes parked files may hold; large files
// live on the process heap until a turn claims them.
const defaultStoredBudget = 512 * 1024 * 1024

func storedAttachmentBudget() int {
	if v := strings.TrimSpace(os.Getenv("QUICKWORK_STAGING_MB")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n * 1024 * 1024
		}
	}
	return defaultStoredBudget
}

func storeAttachment(st StoredAttachment) string {
	id := newUUID()
	attachmentStoreMu.Lock()
	attachmentStore.Store(id, st)
	attachmentStoreMu.Unlock()
	evictStoredAttachments()
	return id
}

// evictStoredAttachments drops the oldest entries once the parked budget is
// exceeded, or entries that have been waiting too long.
func evictStoredAttachments() {
	type entry struct {
		id string
		st StoredAttachment
	}
	var all []entry
	total := 0
	attachmentStore.Range(func(k, v any) bool {
		st, ok := v.(StoredAttachment)
		if !ok {
			return true
		}
		all = append(all, entry{k.(string), st})
		total += len(st.Data)
		return true
	})
	if total <= storedAttachmentBudget() {
		return
	}
	// Oldest first.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].st.CreatedAt.Before(all[j-1].st.CreatedAt); j-- {
			all[j-1], all[j] = all[j], all[j-1]
		}
	}
	for _, e := range all {
		if total <= storedAttachmentBudget() {
			return
		}
		attachmentStore.Delete(e.id)
		total -= len(e.st.Data)
	}
}

func claimAttachment(id string) (StoredAttachment, bool) {
	v, ok := attachmentStore.Load(id)
	if !ok {
		return StoredAttachment{}, false
	}
	st, ok := v.(StoredAttachment)
	return st, ok
}

// sanitizeAttachmentName keeps names safe for the workspace filesystem.
func sanitizeAttachmentName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.Trim(name, ".")
	if name == "" {
		return "attachment"
	}
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	return name
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func attachmentKind(mimeType string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return "image"
	}
	return "file"
}

func extForMime(mimeType string) string {
	m := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch m {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/markdown":
		return ".md"
	case "application/json":
		return ".json"
	}
	return ".bin"
}

func defaultAttachmentName(kind string, index int, mimeType string) string {
	prefix := "file"
	if kind == "image" {
		prefix = "image"
	}
	return fmt.Sprintf("%s_%d%s", prefix, index, extForMime(mimeType))
}

// uploadCache remembers which attachmentId a given content already has on a
// given account. The client's conversation is server-side and long-lived, so it
// never re-uploads history; a stateless proxy replays history every turn, and
// without this cache every turn would push the same bytes again.
var uploadCache sync.Map // "accountID|sha256" -> cachedUpload

type cachedUpload struct {
	AttachmentID string
	MimeType     string
	Size         int
	Name         string
}

func uploadCacheKey(accountID int64, sha string) string {
	return fmt.Sprintf("%d|%s", accountID, sha)
}

func lookupUpload(accountID int64, sha string) (cachedUpload, bool) {
	v, ok := uploadCache.Load(uploadCacheKey(accountID, sha))
	if !ok {
		return cachedUpload{}, false
	}
	c, ok := v.(cachedUpload)
	return c, ok
}

func rememberUpload(accountID int64, sha string, c cachedUpload) {
	uploadCache.Store(uploadCacheKey(accountID, sha), c)
}
