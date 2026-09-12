package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OpenCodeAdapter struct{}

func (OpenCodeAdapter) Alias() string       { return "opencode" }
func (OpenCodeAdapter) Name() string        { return "OpenCode" }
func (OpenCodeAdapter) DisplayName() string { return "OpenCode" }
func (OpenCodeAdapter) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Tools: false, StructuredOutput: false, Usage: false, Vision: false, ModelSelection: true, Sessions: false}
}

func (OpenCodeAdapter) Invoke(req Request) (Invocation, error) {
	msgs := FlattenMessages(req.Messages)
	serialized := serializeMessages(msgs)
	if req.SystemPrompt != "" {
		serialized = "[system] " + req.SystemPrompt + "\n" + serialized
	}
	// Transport: `opencode run` takes the prompt as a positional arg; like the
	// other CLIs there is no supported stdin prompt-replacement, so guard at
	// the real OS ceiling with a clear error instead of a cryptic failure.
	if len([]byte(serialized)) > maxCLIArgLen {
		return Invocation{}, ErrPromptTooLong
	}
	// Prefer machine-readable JSON output over decorative terminal text.
	// Fall back to default format parsing when JSON is unavailable.
	args := []string{
		"run",
		"--format", "json",
	}
	if req.UpstreamModel != "" {
		args = append(args, "-m", req.UpstreamModel)
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, serialized)

	return Invocation{
		Args: args,
		Env: []string{
			"NO_COLOR=1",
		},
		Parse: func(stdout, stderr []byte) (Result, error) {
			if r, ok := parseOpenCodeJSON(stdout); ok {
				return r, nil
			}
			text := strings.TrimSpace(string(stdout))
			if text != "" {
				return Result{Content: text}, nil
			}
			msg := strings.TrimSpace(string(stderr))
			if msg == "" {
				msg = "empty response from OpenCode"
			}
			return Result{}, fmt.Errorf("%s", msg)
		},
	}, nil
}

// StreamInvoke forwards OpenCode's stdout as it arrives. In non-interactive
// runs the answer usually lands as one or a few bursts, but it is real
// incremental output — never a fabricated token-by-token simulation.
func (OpenCodeAdapter) StreamInvoke(req Request) (Invocation, error) {
	inv, err := (OpenCodeAdapter{}).Invoke(req)
	if err != nil {
		return Invocation{}, err
	}
	inv.StreamParse = rawTextStreamParser()
	return inv, nil
}

// parseOpenCodeJSON extracts assistant text from `opencode run --format json`.
// Returns ok=false when stdout is not JSON, letting callers fall back to raw.
func parseOpenCodeJSON(stdout []byte) (Result, bool) {
	s := strings.TrimSpace(string(stdout))
	if s == "" || (s[0] != '{' && s[0] != '[') {
		return Result{}, false
	}
	// Try common shapes: {"text":...}, {"result":...}, {"content":...},
	// {"message":{"content":...}}, or JSONL with one object per line.
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		if txt := openCodeText(m); txt != "" {
			return Result{Content: txt}, true
		}
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var lm map[string]any
		if err := json.Unmarshal([]byte(line), &lm); err != nil {
			continue
		}
		if txt := openCodeText(lm); txt != "" {
			// Last JSONL text wins (accumulate if multiple).
			m = lm
		}
	}
	if m != nil {
		if txt := openCodeText(m); txt != "" {
			return Result{Content: txt}, true
		}
	}
	return Result{}, false
}

func openCodeText(m map[string]any) string {
	for _, k := range []string{"text", "result", "content", "output", "response"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if msg, ok := m["message"].(map[string]any); ok {
		if v, ok := msg["content"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := msg["text"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
