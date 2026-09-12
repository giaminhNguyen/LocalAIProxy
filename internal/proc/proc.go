// Package proc runs one CLI subprocess per request with cancellation,
// timeouts, isolated buffers and Windows process-tree cleanup. No shell is
// involved, so no shell-injection surface exists.
package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"LocalAIProxy/internal/provider"
)

const maxCapture = 8 << 20 // 8 MiB per stream, enough for any real response

// limitedBuffer caps captured output to avoid unbounded RAM growth.
type limitedBuffer struct {
	mu   sync.Mutex
	buf  strings.Builder
	full bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.full {
		room := maxCapture - l.buf.Len()
		if len(p) > room {
			p = p[:room]
			l.full = true
		}
		l.buf.Write(p)
	}
	return len(p), nil
}

func (l *limitedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return []byte(l.buf.String())
}

func (l *limitedBuffer) Text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// Runner implements provider.Runner by spawning child processes.
type Runner struct {
	// HomeDir is used to sanitize captured diagnostics (home paths never go to API clients).
	HomeDir string
}

// Run executes a single invocation. On context cancellation or timeout the
// subprocess tree is killed (Windows: taskkill /T /F).
func (r *Runner) Run(ctx context.Context, inv provider.Invocation) (provider.Result, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, inv.Exec, inv.Args...)
	cmd.Env = append(os.Environ(), inv.Env...)

	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = hideWindowAttr()
	}

	if inv.Stdin != "" {
		cmd.Stdin = strings.NewReader(inv.Stdin)
	}

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("The CLI could not be started. Check that it is installed and available in PATH.")
		e := provider.NewError(provider.ErrProviderProcess, "", msg, 502)
		return provider.Result{}, *e.WithDetails(err.Error())
	}

	killer := newTreeKiller(cmd, ctx)
	killer.watch()

	runErr := cmd.Wait()

	stdoutBytes := stdout.Bytes()
	stderrBytes := stderr.Bytes()

	// Process-level failure wins: classify first, before parsing content.
	if runErr != nil {
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return provider.Result{}, *provider.NewError(provider.ErrProviderTimeout, "", "The CLI took too long and was stopped.", 504).
					WithDetails(sanitize(stderr.Text(), r.HomeDir))
			}
			return provider.Result{}, *provider.NewError(provider.ErrRequestCancelled, "", "Request cancelled.", 499).
				WithDetails(sanitize(stderr.Text(), r.HomeDir))
		}
		if e := classifyExitError(runErr, stderr.Bytes(), r.HomeDir); e != nil {
			// Some CLIs exit non-zero but describe the real failure in stdout
			// JSON (e.g. claude: {"is_error":true,"result":"weekly limit…"}).
			// Let the adapter parse it so clients get the actual reason, not
			// "exited with code 1".
			if e.Code == provider.ErrProviderProcess && len(stdoutBytes) > 0 && inv.Parse != nil {
				if _, perr := inv.Parse(stdoutBytes, stderrBytes); perr != nil {
					detail := perr.Error()
					if pe := classifyExitError(runErr, []byte(detail), r.HomeDir); pe != nil && pe.Code != provider.ErrProviderProcess {
						return provider.Result{}, *pe.WithDetails(detail)
					}
					n := provider.NewError(provider.ErrProviderProcess, "", detail, 502)
					return provider.Result{}, *n.WithDetails(sanitize(detail, r.HomeDir))
				}
			}
			return provider.Result{}, *e
		}
	}

	if inv.Parse != nil {
		res, perr := inv.Parse(stdoutBytes, stderrBytes)
		if perr != nil {
			var e *provider.Error
			if errors.As(perr, &e) {
				pe := *e
				pe.Status = mapStatus(pe.Code)
				return provider.Result{}, pe
			}
			// Adapter surfaced a parse failure -> normalized process error,
			// raw diagnostics kept sanitized for GUI/debug only.
			n := provider.NewError(provider.ErrProviderProcess, "", messageFromText(perr.Error()), 502)
			return provider.Result{}, *n.WithDetails(sanitize(perr.Error(), r.HomeDir))
		}
		return res, nil
	}

	text := strings.TrimSpace(string(stdoutBytes))
	if text == "" {
		n := provider.NewError(provider.ErrProviderProcess, "", "The CLI returned no output.", 502)
		return provider.Result{}, *n.WithDetails(sanitize(stderr.Text(), r.HomeDir))
	}
	_ = start
	return provider.Result{Content: text}, nil
}

// classifyExitError maps a non-zero exit into a normalized error.
func classifyExitError(err error, stderr []byte, home string) *provider.Error {
	msg := sanitize(string(stderr), home)
	low := msg

	code := "?"
	if ee, ok := err.(*exec.ExitError); ok {
		code = fmt.Sprint(ee.ExitCode())
	}

	switch {
	case strings.Contains(low, "rate") && (strings.Contains(low, "limit") || strings.Contains(low, "quota")):
		e := provider.NewError(provider.ErrProviderRateLimited, "", "Rate limit or quota reached.", 429)
		return e.WithDetails(msg)
	case strings.Contains(low, "quota") || strings.Contains(low, "weekly limit") || strings.Contains(low, "monthly limit") || strings.Contains(low, "429"):
		e := provider.NewError(provider.ErrProviderRateLimited, "", "Rate limit or quota reached.", 429)
		return e.WithDetails(msg)
	case strings.Contains(low, "401") || strings.Contains(low, "unauthorized") ||
		(strings.Contains(low, "auth") && strings.Contains(low, "fail")) ||
		strings.Contains(low, "not logged in"):
		e := provider.NewError(provider.ErrProviderAuth, "", "Authentication required. Log in inside the CLI first.", 401)
		return e.WithDetails(msg)
	case strings.Contains(low, "permission"):
		e := provider.NewError(provider.ErrProviderAuth, "", "The CLI needs authorization it cannot request here.", 401)
		return e.WithDetails(msg)
	case strings.Contains(low, "billing") || strings.Contains(low, "insufficient"):
		e := provider.NewError(provider.ErrProviderAuth, "", "The CLI account has a billing or access issue.", 401)
		return e.WithDetails(msg)
	default:
		e := provider.NewError(provider.ErrProviderProcess, "", fmt.Sprintf("The CLI exited with code %s.", code), 502)
		return e.WithDetails(msg)
	}
}

// sanitize scrubs home paths and truncates a diagnostics string. It is the
// ONLY way captured stderr reaches the GUI, never raw.
func sanitize(s string, home string) string {
	if home != "" {
		s = strings.ReplaceAll(s, home, "<user>")
	}
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 4000 {
		r = r[:4000]
	}
	return string(r)
}

// messageFromText produces a user-facing message from an adapter error,
// keeping the technical detail behind Details.
func messageFromText(s string) string {
	return "The CLI produced no usable response."
}

// mapStatus returns a sane HTTP status for a normalized code.
func mapStatus(code string) int {
	switch code {
	case provider.ErrProviderAuth:
		return 401
	case provider.ErrProviderRateLimited, provider.ErrProviderBusy:
		return 429
	case provider.ErrQueueTimeout, provider.ErrProviderTimeout:
		return 504
	case provider.ErrProviderProcess:
		return 502
	case provider.ErrProviderDisabled, provider.ErrModelNotFound,
		provider.ErrInvalidRequest, provider.ErrStreamingNotSupported:
		return 400
	case provider.ErrProviderUnavailable:
		return 503
	}
	return 500
}
