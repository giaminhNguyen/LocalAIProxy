package provider

import (
	"os"
	"strings"
	"testing"
)

func TestSerializeMessages(t *testing.T) {
	inv, err := (ClaudeAdapter{}).Invoke(Request{
		Model: "claude",
		Messages: []Message{
			{Role: RoleSystem, Content: "Be formal"},
			{Role: RoleUser, Content: "Hi"},
			{Role: RoleAssistant, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "[system] Be formal\n[user] Hi\n[assistant] Hello\n"
	if got := inv.Args[1]; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestClaudeInvokeTooLong(t *testing.T) {
	big := strings.Repeat("x", claudeMaxCLILen+1)
	_, err := (ClaudeAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: big}}})
	if err != ErrPromptTooLong {
		t.Fatalf("err = %v, want ErrPromptTooLong", err)
	}
}

func TestClaudeParse(t *testing.T) {
	r, err := parseClaudeJSON([]byte(`{"is_error":false,"result":"hello there","type":"result"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Content != "hello there" {
		t.Fatalf("content = %q", r.Content)
	}

	_, err = parseClaudeJSON([]byte(`{"is_error":true,"result":"boom","type":"error"}`), nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected error 'boom', got %v", err)
	}

	r, err = parseClaudeJSON([]byte("plain text fallback"), nil)
	if err != nil || r.Content != "plain text fallback" {
		t.Fatalf("fallback failed: %v %q", err, r.Content)
	}

	_, err = parseClaudeJSON(nil, []byte("stderr noise"))
	if err == nil {
		t.Fatal("expected stderr error")
	}
}

func TestCodexAdapterShape(t *testing.T) {
	inv, err := (CodexAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(inv.Args, " ")
	for _, want := range []string{"exec", "--json", "--ephemeral", "--sandbox read-only", "-"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %v", want, inv.Args)
		}
	}
	if inv.Stdin != "[user] hi\n" {
		t.Errorf("stdin = %q", inv.Stdin)
	}
	if inv.Parse == nil {
		t.Error("missing parse function")
	}
}

func TestCodexParseLastMessageFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "\\last.txt"
	if err := os.WriteFile(path, []byte("final answer"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := parseCodexJSON(nil, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Content != "final answer" {
		t.Fatalf("content = %q", r.Content)
	}
}

func TestCodexParseJSONLFallback(t *testing.T) {
	stdout := []byte(
		`{"type":"init","some":"stuff"}` + "\n" +
			`{"type":"message","message":{"role":"assistant","content":"second"}}` + "\n" +
			`{"type":"message","message":{"role":"assistant","content":"last one"}}` + "\n",
	)
	r, err := parseCodexJSON(stdout, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Content != "last one" {
		t.Fatalf("content = %q, want last assistant message", r.Content)
	}
}

func TestGeminiOpenCodeShape(t *testing.T) {
	g, err := (GeminiAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if g.Args[0] != "-p" || g.Args[1] != "[user] hi\n" {
		t.Fatalf("gemini args = %v", g.Args)
	}

	o, err := (OpenCodeAdapter{}).Invoke(Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(o.Args, " ")
	if !strings.Contains(joined, "run") || !strings.Contains(joined, "--format json") {
		t.Fatalf("opencode args = %v", o.Args)
	}
	found := false
	for _, e := range o.Env {
		if e == "NO_COLOR=1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("opencode env missing NO_COLOR: %v", o.Env)
	}
}

func TestAsError(t *testing.T) {
	wrapped := NewError(ErrProviderAuth, "claude", "bad login", 401)
	var out *Error
	if !AsError(wrapped, &out) {
		t.Fatal("AsError failed on direct *Error")
	}
	if out.Code != ErrProviderAuth {
		t.Fatalf("code = %q", out.Code)
	}

	var out2 *Error
	if AsError(nil, &out2) {
		t.Fatal("AsError(nil) should fail")
	}
}
