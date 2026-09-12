package provider

import (
	"fmt"
	"strings"
)

type GeminiAdapter struct{}

func (GeminiAdapter) Alias() string       { return "gemini" }
func (GeminiAdapter) Name() string        { return "Gemini CLI" }
func (GeminiAdapter) DisplayName() string { return "Gemini CLI" }

func (GeminiAdapter) Invoke(req Request) (Invocation, error) {
	// Best-effort invocation. The Google Gemini CLI for coding (gemini / antigravity)
	// supports `-p` print mode with stdout text output. If the installed CLI
	// uses a different interface, the Parse function handles graceful fallback.
	serialized := serializeMessages(req.Messages)

	args := []string{
		"-p", serialized,
	}

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
