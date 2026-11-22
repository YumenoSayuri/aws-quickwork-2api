package proxy

import (
	"os"
	"strings"
	"testing"
)

// The override notice is opt-in: by default the caller instructions are the
// whole system text. Measured against the live service, the notice does not
// change the assistant identity and the model flags it as prompt injection.
func TestSystemPromptDefaultsToCallerInstructions(t *testing.T) {
	os.Unsetenv("QUICKWORK_FORCE_OVERRIDE")
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
	os.Unsetenv("QUICKWORK_FORCE_OVERRIDE")
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
