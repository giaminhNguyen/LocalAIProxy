package provider

import (
	"fmt"
	"strings"
)

type OpenCodeAdapter struct{}

func (OpenCodeAdapter) Alias() string       { return "opencode" }
func (OpenCodeAdapter) Name() string        { return "OpenCode" }
func (OpenCodeAdapter) DisplayName() string { return "OpenCode" }

func (OpenCodeAdapter) Invoke(req Request) (Invocation, error) {
	serialized := serializeMessages(req.Messages)
	args := []string{
		"run",
		"--format", "default",
		serialized,
	}

	return Invocation{
		Args: args,
		Env: []string{
			"NO_COLOR=1",
		},
		Parse: func(stdout, stderr []byte) (Result, error) {
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
