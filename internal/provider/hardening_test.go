package provider

import (
	"strings"
	"testing"
)

func TestCodexTempFilesUnique(t *testing.T) {
	inv1, err := (CodexAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	inv2, err := (CodexAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	p1 := lastMsgPath(inv1.Args)
	p2 := lastMsgPath(inv2.Args)
	if p1 == "" || p2 == "" {
		t.Fatal("expected --output-last-message paths")
	}
	if p1 == p2 {
		t.Fatalf("temp files must be unique per request, got same %q", p1)
	}
	if strings.Contains(p1, "_") && isPIDOnly(p1) {
		t.Fatalf("must not use PID-only naming: %q", p1)
	}
}

func lastMsgPath(args []string) string {
	for i, a := range args {
		if a == "--output-last-message" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func isPIDOnly(p string) bool {
	// Old bug: localaiproxy_codex_<pid>.txt — no randomness.
	// New paths contain a random suffix from os.CreateTemp.
	return strings.Contains(p, "localaiproxy_codex_") && !strings.Contains(p, "*") && len(p) < 60
}

func TestCapabilitiesHonest(t *testing.T) {
	adapters := []Adapter{ClaudeAdapter{}, CodexAdapter{}, GeminiAdapter{}, OpenCodeAdapter{}}
	for _, a := range adapters {
		caps := a.Capabilities()
		if !caps.Streaming {
			t.Errorf("%s: streaming should be true (all backends stream stdout)", a.Alias())
		}
		if caps.Tools || caps.Vision || caps.Sessions {
			t.Errorf("%s: must not advertise unsupported tools/vision/sessions", a.Alias())
		}
		if !caps.ModelSelection {
			t.Errorf("%s: should support model selection", a.Alias())
		}
		if caps.StructuredOutput {
			t.Errorf("%s: must not claim structured output", a.Alias())
		}
	}
}

func TestUpstreamModelFlag(t *testing.T) {
	inv, err := (ClaudeAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, UpstreamModel: "claude-sonnet-4"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(inv.Args, "--model", "claude-sonnet-4") {
		t.Fatalf("claude upstream flag missing: %v", inv.Args)
	}
	inv2, err := (CodexAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, UpstreamModel: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(inv2.Args, "--model", "gpt-5") {
		t.Fatalf("codex upstream flag missing: %v", inv2.Args)
	}
	inv3, err := (OpenCodeAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, UpstreamModel: "opencode-sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(inv3.Args, "-m", "opencode-sonnet") {
		t.Fatalf("opencode upstream flag missing: %v", inv3.Args)
	}
}

func containsArg(args []string, flag, val string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}
	return false
}

func TestFlattenContentParts(t *testing.T) {
	m := Message{Role: RoleUser, ContentParts: []ContentPart{{Type: "text", Text: "hello"}, {Type: "text", Text: "world"}}}
	if got := FlattenContent(m); got != "hello\nworld" {
		t.Fatalf("flatten = %q", got)
	}
	if IsNonRetryable(ErrProviderAuth) != true || IsNonRetryable(ErrProviderRateLimited) != true || IsNonRetryable(ErrRequestCancelled) != true {
		t.Fatal("auth/quota/cancel must be non-retryable")
	}
	if IsNonRetryable(ErrProviderProcess) {
		t.Fatal("process errors may be retryable conservatively")
	}
}
