package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CodexAdapter struct{}

func (CodexAdapter) Alias() string       { return "codex" }
func (CodexAdapter) Name() string        { return "OpenAI Codex CLI" }
func (CodexAdapter) DisplayName() string { return "Codex" }

func (CodexAdapter) Invoke(req Request) (Invocation, error) {
	serialized := serializeMessages(req.Messages)

	args := []string{
		"exec",
		"--json",
		"--ephemeral",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"-", // prompt via stdin
	}

	tempDir := os.TempDir()
	lastMsgFile := filepath.Join(tempDir, fmt.Sprintf("localaiproxy_codex_%d.txt", os.Getpid()))
	args = append(args, "--output-last-message", lastMsgFile)

	return Invocation{
		Args:  args,
		Stdin: serialized,
		Parse: func(stdout, stderr []byte) (Result, error) {
			return parseCodexJSON(stdout, stderr, lastMsgFile)
		},
	}, nil
}

// codexJSONL tracks codex exec --json events, specifically the final message.
type codexEvent struct {
	Type    string    `json:"type"`
	Message *codexMsg `json:"message,omitempty"`
}

type codexMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func parseCodexJSON(stdout, stderr []byte, lastMsgPath string) (Result, error) {
	// Prefer the explicit output-last-message file when present.
	if lastMsgPath != "" {
		if b, err := os.ReadFile(lastMsgPath); err == nil && len(bytes.TrimSpace(b)) > 0 {
			_ = os.Remove(lastMsgPath)
			return Result{Content: strings.TrimSpace(string(b))}, nil
		}
		_ = os.Remove(lastMsgPath)
	}

	// Fall back to scanning JSONL events for the last assistant message.
	var lastAssistant string
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var evt codexEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "message" && evt.Message != nil && evt.Message.Role == "assistant" {
			lastAssistant = evt.Message.Content
		}
	}
	if lastAssistant != "" {
		return Result{Content: lastAssistant}, nil
	}

	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = "empty response from Codex"
	}
	return Result{}, fmt.Errorf("%s", msg)
}
