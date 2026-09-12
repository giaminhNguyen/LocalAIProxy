package provider

import (
	"fmt"
	"strings"
)

type GeminiAdapter struct{}

func (GeminiAdapter) Alias() string       { return "gemini" }
func (GeminiAdapter) Name() string        { return "Gemini CLI" }
func (GeminiAdapter) DisplayName() string { return "Gemini CLI" }
func (GeminiAdapter) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Tools: false, StructuredOutput: false, Usage: false, Vision: false, ModelSelection: true, Sessions: false}
}

// maxCLIArgLen is the shared Windows command-line safety ceiling (see claude.go).
const maxCLIArgLen = 30000

func (GeminiAdapter) Invoke(req Request) (Invocation, error) {
	msgs := FlattenMessages(req.Messages)
	serialized := serializeMessages(msgs)
	if req.SystemPrompt != "" {
		serialized = "[system] " + req.SystemPrompt + "\n" + serialized
	}

	// Transport: `gemini -p` takes the prompt as an arg (stdin is only
	// supplementary context in the Gemini CLI, not a prompt replacement), so
	// CLI-arg remains the transport with an explicit guard at the real OS
	// ceiling instead of a cryptic exec failure.
	if len([]byte(serialized)) > maxCLIArgLen {
		return Invocation{}, ErrPromptTooLong
	}
	args := []string{
		"-p", serialized,
	}
	if req.UpstreamModel != "" {
		args = append(args, "--model", req.UpstreamModel)
	}
	args = append(args, req.ExtraArgs...)

	return Invocation{
		Args: args,
		Parse: func(stdout, stderr []byte) (Result, error) {
			text := strings.TrimSpace(string(stdout))
			if text != "" {
				return Result{Content: text}, nil
			}
			msg := strings.TrimSpace(string(stderr))
			if msg == "" {
				msg = "empty response from Gemini"
			}
			return Result{}, fmt.Errorf("%s", msg)
		},
	}, nil
}

// StreamInvoke forwards the CLI's stdout as it arrives. Gemini -p prints a
// plain-text answer; whether it tokenizes live depends on the installed CLI and
// whether the shell detected a TTY, so the deltas may arrive in a single burst.
func (GeminiAdapter) StreamInvoke(req Request) (Invocation, error) {
	inv, err := (GeminiAdapter{}).Invoke(req)
	if err != nil {
		return Invocation{}, err
	}
	inv.StreamParse = rawTextStreamParser()
	return inv, nil
}
