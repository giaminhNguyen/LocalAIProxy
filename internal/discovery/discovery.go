// Package discovery finds the four supported CLIs on the local machine and
// probes their version + authentication status using each CLI's official
// status command. No AI request is ever made here, so opening the app never
// consumes quota.
package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// AuthState is the honest answer to "is this CLI logged in locally?".
type AuthState string

const (
	AuthDetected AuthState = "detected"
	AuthRequired AuthState = "required"
	AuthUnknown  AuthState = "unknown"
)

// Info captures everything discovery knows about one provider.
type Info struct {
	Alias      string
	Name       string
	Executable string
	Installed  bool
	Version    string
	Auth       AuthState
}

var executors = []string{
	"claude",
	"gemini",
	"codex",
	"opencode",
	"antigravity",
}

// ExecNames returns CLI executable names to probe for an alias.
func ExecNames(alias string) []string {
	switch alias {
	case "claude":
		return []string{"claude"}
	case "codex":
		return []string{"codex"}
	case "gemini":
		return []string{"gemini", "antigravity"}
	case "opencode":
		return []string{"opencode"}
	}
	return []string{alias}
}

// FindExecutable resolves a CLI on PATH (or known fallback dirs).
func FindExecutable(alias string) string {
	for _, name := range ExecNames(alias) {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			return p
		}
	}
	if alias == "opencode" {
		// opencode installs to %USERPROFILE%\.opencode\bin
		if p, err := exec.LookPath("opencode.exe"); err == nil {
			return p
		}
	}
	return ""
}

// Probe runs detection for a single provider and returns a snapshot.
func Probe(ctx context.Context, alias, name string) Info {
	info := Info{Alias: alias, Name: name}
	exe := FindExecutable(alias)
	if exe == "" {
		return info
	}
	info.Executable = exe
	info.Installed = true
	info.Version = Version(ctx, exe)
	if info.Version == "" {
		info.Version = "unknown"
	}
	info.Auth = AuthStatus(ctx, alias, exe)
	return info
}

// Version runs `<exe> --version` with a short timeout.
func Version(ctx context.Context, exe string) string {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes.TrimSpace(out)))
}

// AuthStatus uses each CLI's official local status command. Never issues an
// AI request, so probing auth cannot consume quota or change state.
func AuthStatus(ctx context.Context, alias, exe string) AuthState {
	cctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	switch alias {
	case "claude":
		cmd := exec.CommandContext(cctx, exe, "auth", "status")
		out, err := cmd.Output()
		if err != nil {
			return AuthUnknown
		}
		var st struct {
			LoggedIn bool `json:"loggedIn"`
		}
		if json.Unmarshal(out, &st) == nil {
			if st.LoggedIn {
				return AuthDetected
			}
			return AuthRequired
		}
		return AuthUnknown

	case "codex":
		cmd := exec.CommandContext(cctx, exe, "login", "status")
		if err := cmd.Run(); err != nil {
			return AuthRequired
		}
		return AuthDetected

	case "gemini":
		cmd := exec.CommandContext(cctx, exe, "auth", "status")
		out, err := cmd.Output()
		_ = out
		if err != nil {
			return AuthRequired
		}
		return AuthDetected

	case "opencode":
		cmd := exec.CommandContext(cctx, exe, "auth", "list")
		out, err := cmd.Output()
		if err != nil {
			return AuthUnknown
		}
		if strings.Contains(string(out), "0 credentials") {
			return AuthRequired
		}
		return AuthDetected
	}
	return AuthUnknown
}

// NotUsed declares executors for documentation purposes.
var _ = executors
