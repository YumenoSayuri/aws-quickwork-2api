package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AgentSpec is a custom chat agent definition. Instructions is where the
// service stores the agent's system prompt; it is the only lever that replaces
// the built-in assistant identity, because the desktop user_message frame has
// no per-turn system_prompt field.
type AgentSpec struct {
	Name         string
	Description  string
	Instructions string
}

// CreateAgent creates a draft agent carrying spec and returns its id.
//
// The agent lifecycle is the desktop command set, and every command is also
// reachable as plain HTTP through the runtime's /api/ws-rpc endpoint, which the
// data plane proxies via the qbs-event invokeAgent envelope. Publishing runs
// longer than the relay timeout, so this returns a draft: chat turns reference
// it with chat_agent_id plus use_draft.
func (u *Upstream) CreateAgent(ctx context.Context, sess *Session, spec AgentSpec) (string, error) {
	frames, err := u.agentRPC(ctx, sess, "create_agent_draft", map[string]any{"new_agent": true})
	if err != nil {
		return "", err
	}
	agentID := ""
	for _, f := range frames {
		if id := frameString(f, "agent_id", "agentId"); id != "" {
			agentID = id
			break
		}
		if draft, ok := f["draft"].(map[string]any); ok {
			if id, _ := draft["agent_id"].(string); id != "" {
				agentID = id
				break
			}
		}
	}
	if agentID == "" {
		return "", fmt.Errorf("agent service did not return an agent id: %s", compactFrames(frames))
	}

	desc := strings.TrimSpace(spec.Description)
	if desc == "" {
		desc = "Custom assistant"
	}
	if _, err := u.agentRPC(ctx, sess, "update_chat_agent_config", map[string]any{
		"agent_id":     agentID,
		"target":       "draft",
		"name":         spec.Name,
		"description":  desc,
		"instructions": spec.Instructions,
	}); err != nil {
		return "", fmt.Errorf("save agent instructions: %w", err)
	}
	return agentID, nil
}

// agentRPC sends one desktop command through /api/ws-rpc and returns the frames
// it answered with.
func (u *Upstream) agentRPC(ctx context.Context, sess *Session, kind string, payload map[string]any) ([]map[string]any, error) {
	body, err := json.Marshal(map[string]any{"type": kind, "payload": payload})
	if err != nil {
		return nil, err
	}
	env, err := json.Marshal(map[string]any{
		"eventType": "invokeAgent",
		"eventBody": map[string]any{
			"invokeAgent": map[string]any{
				"path":        "api/ws-rpc",
				"queryParams": map[string]any{"_method": "POST"},
				"body":        json.RawMessage(body),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	url := tenantBaseURL(sess) + "/integration/quick-work/qbs-event"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sess.JWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := attachmentClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent rpc %s: HTTP %d %s", kind, resp.StatusCode, truncate(string(raw), 300))
	}

	var outer struct {
		ErrorMessage  string `json:"errorMessage"`
		EventResponse struct {
			InvokeAgentResponse json.RawMessage `json:"invokeAgentResponse"`
		} `json:"eventResponse"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("decode agent rpc: %w", err)
	}
	if outer.ErrorMessage != "" {
		return nil, fmt.Errorf("%s", outer.ErrorMessage)
	}
	inner := outer.EventResponse.InvokeAgentResponse
	if len(inner) == 0 {
		return nil, fmt.Errorf("agent rpc %s: empty response", kind)
	}
	// invokeAgentResponse is a JSON string holding status/headers/body, and body
	// is another JSON string holding the frames.
	var envelope struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(inner, &envelope); err != nil {
		var asString string
		if json.Unmarshal(inner, &asString) != nil {
			return nil, fmt.Errorf("decode invokeAgentResponse: %w", err)
		}
		if err := json.Unmarshal([]byte(asString), &envelope); err != nil {
			return nil, fmt.Errorf("decode invokeAgentResponse body: %w", err)
		}
	}
	bodyRaw := envelope.Body
	if len(bodyRaw) > 0 && bodyRaw[0] == '"' {
		var s string
		if err := json.Unmarshal(bodyRaw, &s); err == nil {
			bodyRaw = []byte(s)
		}
	}
	var framesDoc struct {
		Frames []map[string]any `json:"frames"`
	}
	if err := json.Unmarshal(bodyRaw, &framesDoc); err != nil {
		return nil, fmt.Errorf("decode agent frames: %w (status %d)", err, envelope.Status)
	}
	for _, f := range framesDoc.Frames {
		if t, _ := f["type"].(string); strings.Contains(t, "error") {
			text := frameString(f, "error", "message")
			if text == "" {
				text = compactFrames([]map[string]any{f})
			}
			return framesDoc.Frames, fmt.Errorf("%s", text)
		}
	}
	return framesDoc.Frames, nil
}

func frameString(f map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := f[k].(string); ok && v != "" {
			return v
		}
	}
	if p, ok := f["payload"].(map[string]any); ok {
		for _, k := range keys {
			if v, ok := p[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// compactFrames renders frames for an error message.
func compactFrames(frames []map[string]any) string {
	raw, err := json.Marshal(frames)
	if err != nil {
		return ""
	}
	return truncate(string(raw), 500)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
