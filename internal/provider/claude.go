package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"unicode/utf8"
)

const claudeMaxCLILen = 30000 // Windows CreateProcess ceiling is 32767 incl. exe+flags; keep headroom

type ClaudeAdapter struct{}

func (ClaudeAdapter) Alias() string       { return "claude" }
func (ClaudeAdapter) Name() string        { return "Claude Code" }
func (ClaudeAdapter) DisplayName() string { return "Claude Code" }
func (ClaudeAdapter) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Tools: false, StructuredOutput: false, Usage: true, Vision: false, ModelSelection: true, Sessions: false}
}

func (ClaudeAdapter) Invoke(req Request) (Invocation, error) {
	msgs := FlattenMessages(req.Messages)
	serialized := serializeMessages(msgs)
	if req.SystemPrompt != "" {
		serialized = "[system] " + req.SystemPrompt + "\n" + serialized
	}
	// Preferred transport: stdin is not reliably supported by `claude -p`,
	// so keep CLI-arg transport but enforce the real Windows command-line
	// ceiling (not an artificial one). Long conversations fail fast with a
	// clear error instead of truncating or breaking at the OS level.
	if len([]byte(serialized)) > claudeMaxCLILen {
		return Invocation{}, ErrPromptTooLong
	}
	args := []string{
		"-p", serialized,
		"--output-format", "json",
		"--permission-mode", "plan",
		"--permission-prompts", "none",
	}
	if req.UpstreamModel != "" {
		args = append(args, "--model", req.UpstreamModel)
	}
	args = append(args, req.ExtraArgs...)
	return Invocation{
		Args:  args,
		Parse: parseClaudeJSON,
	}, nil
}

// StreamInvoke uses `--output-format stream-json`, which emits one JSON event
// per line as the model generates (content_block_delta carries text_delta).
// This is Claude Code's genuine native token streaming.
func (ClaudeAdapter) StreamInvoke(req Request) (Invocation, error) {
	msgs := FlattenMessages(req.Messages)
	serialized := serializeMessages(msgs)
	if req.SystemPrompt != "" {
		serialized = "[system] " + req.SystemPrompt + "\n" + serialized
	}
	if len([]byte(serialized)) > claudeMaxCLILen {
		return Invocation{}, ErrPromptTooLong
	}
	args := []string{
		"-p", serialized,
		"--output-format", "stream-json",
		"--permission-mode", "plan",
		"--permission-prompts", "none",
	}
	if req.UpstreamModel != "" {
		args = append(args, "--model", req.UpstreamModel)
	}
	args = append(args, req.ExtraArgs...)
	return Invocation{
		Args:        args,
		StreamParse: newClaudeStreamParser(),
	}, nil
}

// claudeStreamEvent is the subset of the stream-json event envelope that
// matters for text forwarding.
type claudeStreamEvent struct {
	Type  string `json:"type"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Event *struct {
		Type  string `json:"type"`
		Delta *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta,omitempty"`
	} `json:"event,omitempty"`
}

// newClaudeStreamParser returns a StreamParseLine over stream-json output.
// Events are usually one JSON object per line; the parser also tolerates
// pretty-printed events by buffering until the buffer is valid JSON.
func newClaudeStreamParser() StreamParseLine {
	var pending []byte
	return func(line string) (string, bool, error) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return "", false, nil
		}
		pending = append(pending, trimmed...)
		if !json.Valid(pending) {
			return "", false, nil // wait for the rest of a multi-line event
		}
		buf := pending
		pending = nil
		var ev claudeStreamEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return "", false, err
		}
		if ev.Error != nil && ev.Error.Message != "" {
			return "", false, fmt.Errorf("%s", ev.Error.Message)
		}
		typ := ev.Type
		if typ == "stream_event" && ev.Event != nil {
			typ = ev.Event.Type
		}
		switch typ {
		case "content_block_delta":
			if ev.Event != nil && ev.Event.Delta != nil {
				return ev.Event.Delta.Text, false, nil
			}
		case "message_stop", "result":
			return "", true, nil
		default:
			// message_start, content_block_start, message_delta, usage, ... carry
			// no assistant content.
		}
		return "", false, nil
	}
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
