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
