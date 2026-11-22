package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleAgents creates a custom agent whose instructions replace the built-in
// assistant identity, and returns its id. Pass the id back either in the
// X-Quickwork-Agent header or in the config as agent_id.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	var body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
		Prompt       string `json:"prompt"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	instructions := strings.TrimSpace(firstNonEmpty(body.Instructions, body.Prompt))
	if instructions == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "instructions is required")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Custom Agent"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	var lastErr error
	for range s.maxAttempts(ctx) {
		acct, err := s.accounts.Next(ctx)
		if err != nil {
			lastErr = err
			break
		}
		sess := SessionFromAccount(acct, s.cfg)
		if err := sess.validate(); err != nil {
			s.accounts.ReportFailure(ctx, acct.ID, err)
			lastErr = err
			continue
		}
		id, err := s.upstream.CreateAgent(ctx, sess, AgentSpec{
			Name:         name,
			Description:  body.Description,
			Instructions: instructions,
		})
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"agent_id":     id,
				"name":         name,
				"instructions": instructions,
			})
			return
		}
		lastErr = err
		s.accounts.ReportFailure(ctx, acct.ID, err)
	}
	writeOpenAIError(w, http.StatusBadGateway, "agent_create_failed", lastErr.Error())
}
