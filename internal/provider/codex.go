package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type CodexAdapter struct{}

func (CodexAdapter) Alias() string       { return "codex" }
func (CodexAdapter) Name() string        { return "OpenAI Codex CLI" }
func (CodexAdapter) DisplayName() string { return "Codex" }
func (CodexAdapter) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Tools: false, StructuredOutput: false, Usage: false, Vision: false, ModelSelection: true, Sessions: false}
}

func (CodexAdapter) Invoke(req Request) (Invocation, error) {
	msgs := FlattenMessages(req.Messages)
	serialized := serializeMessages(msgs)
	if req.SystemPrompt != "" {
		serialized = "[system] " + req.SystemPrompt + "\n" + serialized
	}

	args := []string{
		"exec",
		"--json",
		"--ephemeral",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"-", // prompt via stdin (avoids Windows command-line limits)
	}
	if req.UpstreamModel != "" {
		args = append(args, "--model", req.UpstreamModel)
	}

	// ponytail: per-request unique temp file via os.CreateTemp; never PID-only.
	tmp, err := os.CreateTemp("", "localaiproxy_codex_*.txt")
	lastMsgFile := ""
	if err == nil {
		lastMsgFile = tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(lastMsgFile) // let CLI recreate it; path stays unique
		args = append(args, "--output-last-message", lastMsgFile)
	}
	args = append(args, req.ExtraArgs...)

	return Invocation{
		Args:  args,
		Stdin: serialized,
		Parse: func(stdout, stderr []byte) (Result, error) {
			return parseCodexJSON(stdout, stderr, lastMsgFile)
		},
	}, nil
}

// StreamInvoke shares the standard exec invocation; codex emits JSONL where the
// assistant answer arrives as a "message" event. We extract that content and
// forward it as a single (honest) delta when it lands — codex does not tokenize
// incrementally, so this is still a burst rather than word-by-word streaming.
func (CodexAdapter) StreamInvoke(req Request) (Invocation, error) {
	inv, err := (CodexAdapter{}).Invoke(req)
	if err != nil {
		return Invocation{}, err
	}
	inv.StreamParse = newCodexStreamParser()
	return inv, nil
}

// newCodexStreamParser tracks the latest assistant message content and emits
// only the newly received slice, so repeated events never duplicate text.
func newCodexStreamParser() StreamParseLine {
	var last string
	return func(line string) (string, bool, error) {
		line = strings.TrimSpace(line)
		if line == "" {
			return "", false, nil
		}
		var evt codexEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return "", false, nil
		}
		if evt.Type != "message" || evt.Message == nil || evt.Message.Role != "assistant" {
			return "", false, nil
		}
		content := evt.Message.Content
		if content == "" {
			return "", false, nil
		}
		var delta string
		if strings.HasPrefix(content, last) {
			delta = content[len(last):]
		} else {
			delta = content
		}
		last = content
		return delta, false, nil
	}
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
