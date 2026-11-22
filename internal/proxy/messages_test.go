package proxy

import (
	"strings"
	"testing"
)

// The override notice is opt-in now: measured against the live service it does
// not change the assistant identity, and the model reports it as a prompt
// injection attempt. Tool bridging works without it.
func TestSystemPromptDefaultsToCallerInstructions(t *testing.T) {
	got := systemPrompt([]Message{
		{Role: "system", Content: "You are a coding agent."},
		{Role: "developer", Content: "Be terse."},
	})
	want := "You are a coding agent.\n\nBe terse."
	if got != want {
		t.Fatalf("client instructions only, got:\n%q", got)
	}
}

func TestSystemPromptEmptyWithoutClientInstructions(t *testing.T) {
	if got := systemPrompt([]Message{{Role: "user", Content: "hi"}}); got != "" {
		t.Fatalf("expected empty system prompt, got:\n%q", got)
	}
}

func TestSystemPromptOverrideOptIn(t *testing.T) {
	t.Setenv("QUICKWORK_FORCE_OVERRIDE", "1")
	got := systemPrompt([]Message{{Role: "system", Content: "You are a coding agent."}})
	if !strings.HasPrefix(got, "You are a coding agent.\n\n") {
		t.Fatalf("client instructions must come first:\n%s", got)
	}
	if !strings.HasSuffix(got, upstreamPromptOverride) {
		t.Fatalf("override must be last when opted in:\n%s", got)
	}
}
