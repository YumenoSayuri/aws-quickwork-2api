// Package wui serves the browser management console for accounts.
package wui

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const cookieName = "qwui"

// Password is a fixed-length credential holding the console secret hash.
type Password struct {
	sum [32]byte
}

func NewPassword(secret string) *Password {
	p := &Password{}
	p.sum = sha256.Sum256([]byte(secret))
	return p
}

func (p *Password) matches(candidate string) bool {
	sum := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(sum[:], p.sum[:]) == 1
}

// sessionTable keeps issued browser sessions in memory.
type sessionTable struct {
	mu     sync.Mutex
	tokens map[string]time.Time
	ttl    time.Duration
}

func newSessions(ttl time.Duration) *sessionTable {
	return &sessionTable{tokens: make(map[string]time.Time), ttl: ttl}
}

func (s *sessionTable) issue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.tokens {
		if now.After(exp) {
			delete(s.tokens, k)
		}
	}
	s.tokens[token] = now.Add(s.ttl)
	return token, nil
}

func (s *sessionTable) valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *sessionTable) drop(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

// guard wraps the console handlers with cookie authentication.
type guard struct {
	pw   *Password
	sess *sessionTable
}

func (g *guard) authorised(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return g.sess.valid(c.Value)
}

func (g *guard) require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !g.authorised(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorised")
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}

func writeOK(w http.ResponseWriter, v any) {
	payload := map[string]any{"ok": true}
	if m, ok := v.(map[string]any); ok {
		for k, val := range m {
			payload[k] = val
		}
	} else if v != nil {
		payload["data"] = v
	}
	writeJSON(w, http.StatusOK, payload)
}
