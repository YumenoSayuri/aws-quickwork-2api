package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The upstream rejects any user_message frame whose decoded JSON payload
// exceeds 64 KB (observed as {"code":"FrameTooLarge"} with
// "failed_frame_type":"user_message"). Keep a margin for the fixed fields.
const defaultPayloadBudget = 62000

// payloadBudget returns the byte budget for one user_message payload.
func payloadBudget() int {
	if v := strings.TrimSpace(os.Getenv("QUICKPROXY_MAX_PAYLOAD")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1024 {
			return n
		}
	}
	return defaultPayloadBudget
}

// envelopeSlack is the head-room reserved for the fixed payload fields plus
// the trailing truncation note.
const envelopeSlack = 900

// joinSystem mirrors how the handler merges the caller's system prompt with the
// tool bridge prompt.
func joinSystem(systemPromptText, bridgePrompt string) string {
	if strings.TrimSpace(bridgePrompt) == "" {
		return systemPromptText
	}
	return systemPromptText + "\n\n" + bridgePrompt
}

// payloadBytes reports the size of the user_message body the upstream would
// receive for the given text and system prompt. Only the variable parts are
// measured exactly; the fixed envelope adds a few hundred bytes, which the
// budget margin covers.
func payloadBytes(text, systemPrompt string) (total, textBytes, systemBytes int) {
	payload := map[string]any{
		"text":               text,
		"send_origin":        "programmatic",
		"had_physical_input": false,
		"surfaceType":        "QUICK_WORK_DESKTOP_WINDOWS",
		"outpostId":          "",
		"appVersion":         "1.2.4660",
		"traceId":            "00000000-0000-0000-0000-000000000000",
		"message_id":         "00000000-0000-0000-0000-000000000000",
		"thread_id":          "00000000-0000-0000-0000-000000000000",
		"agent_mode":         "smart",
		"user_timezone":      "UTC",
		"user_locale":        "en-US",
		"chat_agent_id":      nil,
		"auto_routing":       false,
		"web_search_enabled": false,
		"pre_approved_tools": []any{},
		"preload_skills":     []any{},
		"panel_edits":        []any{},
		"custom_prompt":      nil,
	}
	if strings.TrimSpace(systemPrompt) != "" {
		payload["system_prompt"] = systemPrompt
	}
	body := map[string]any{"type": "user_message", "payload": payload}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, len(text), len(systemPrompt)
	}
	return len(raw), len(text), len(systemPrompt)
}

// formatPromptBudget formats the transcript and, when the result exceeds
// budget bytes, drops the oldest non-system turns until it fits. It returns the
// prompt and how many messages were dropped.
func formatPromptBudget(msgs []Message, budget int) (string, int) {
	prompt, err := formatPrompt(msgs)
	if err != nil {
		return "", 0
	}
	if budget <= 0 || len(prompt) <= budget {
		return prompt, 0
	}

	work := append([]Message(nil), msgs...)
	dropped := 0
	for len(prompt) > budget {
		idx := -1
		for i := 0; i < len(work); i++ {
			role := strings.ToLower(strings.TrimSpace(work[i].Role))
			if role == "system" || role == "developer" {
				continue
			}
			// Never drop the final message: it carries the live turn.
			if i == len(work)-1 {
				break
			}
			idx = i
			break
		}
		if idx < 0 {
			break
		}
		work = append(work[:idx], work[idx+1:]...)
		dropped++
		next, err := formatPrompt(work)
		if err != nil {
			break
		}
		prompt = next
	}
	return prompt, dropped
}

// payloadTooLargeError explains exactly which part blew the budget, so a client
// can act on it instead of seeing an opaque upstream failure.
func payloadTooLargeError(total, budget int, prompt, systemPrompt string, dropped int) error {
	_, textBytes, systemBytes := payloadBytes(prompt, systemPrompt)
	hint := "reduce the conversation history"
	if systemBytes > budget/2 {
		hint = "reduce the number of tools or their schema size"
	}
	msg := fmt.Sprintf(
		"request payload is %d bytes, over the upstream limit of %d bytes (text %d + system_prompt %d)",
		total, budget, textBytes, systemBytes)
	if dropped > 0 {
		msg += fmt.Sprintf("; %d oldest message(s) were already dropped", dropped)
	}
	return fmt.Errorf("%s; %s", msg, hint)
}
