package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"unicode/utf8"
)

const claudeMaxCLILen = 15000

type ClaudeAdapter struct{}

func (ClaudeAdapter) Alias() string       { return "claude" }
func (ClaudeAdapter) Name() string        { return "Claude Code" }
func (ClaudeAdapter) DisplayName() string { return "Claude Code" }

func (ClaudeAdapter) Invoke(req Request) (Invocation, error) {
	serialized := serializeMessages(req.Messages)
	if len([]byte(serialized)) > claudeMaxCLILen {
		return Invocation{}, ErrPromptTooLong
	}
	args := []string{
		"-p", serialized,
		"--output-format", "json",
		"--permission-mode", "plan",
		"--permission-prompts", "none",
	}
	return Invocation{
		Args:  args,
		Parse: parseClaudeJSON,
	}, nil
}

// claudeJSON mirrors the shape of claude --output-format json.
type claudeJSON struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Type    string `json:"type"`
}

func parseClaudeJSON(stdout, stderr []byte) (Result, error) {
	if len(stdout) == 0 {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = "empty response from Claude"
		}
		return Result{}, fmt.Errorf("%s", msg)
	}
	// Some versions emit trailing newlines; trim valid JSON region
	out := bytes.TrimSpace(stdout)
	var cj claudeJSON
	if err := json.Unmarshal(out, &cj); err != nil {
		// stdout might be partial or malformed; fall back to raw text
		text := strings.TrimSpace(string(out))
		if text == "" {
			return Result{}, fmt.Errorf("malformed JSON from Claude: %v", err)
		}
		return Result{Content: text}, nil
	}
	if cj.IsError {
		return Result{}, fmt.Errorf("%s", strings.TrimSpace(cj.Result))
	}
	return Result{Content: cj.Result}, nil
}

// serializeMessages turns a multi-message conversation into a deterministic
// single prompt string that Claude Code accepts in -p mode.
// Format is simple and readable:
//
//	[system] <content>
//	[user] <content>
//	[assistant] <content>
//	[user] <content>
func serializeMessages(messages []Message) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
	}
	return sb.String()
}

// sanitizeRedactPath strips the Windows user home directory from strings
// to prevent leaking machine-local details to normal API clients.
func sanitizeRedactPath(s string, homeDir string) string {
	if homeDir == "" || len(s) == 0 {
		return s
	}
	return strings.ReplaceAll(s, homeDir, "<user>")
}

// sanitizeStderr sanitizes a stderr chunk: truncates to a safe length and
// strips absolute Windows paths. It never returns raw stderr as primary output.
func sanitizeStderr(b []byte, homeDir string) string {
	s := sanitizeRedactPath(string(b), homeDir)
	if utf8.RuneCountInString(s) > 2000 {
		s = string([]rune(s)[:2000]) + "…"
	}
	return strings.TrimSpace(s)
}

// fakeID returns a random chatcmpl-<id> style string.
func fakeID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return fmt.Sprintf("chatcmpl-%s", string(b))
}
