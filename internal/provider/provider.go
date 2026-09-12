// Package provider defines the unified request types and the four CLI adapters.
// LocalAIProxy never manages provider credentials: each adapter invokes the
// CLI that already owns that provider's local login/session.
package provider

import (
	"context"
	"fmt"
)

// Role is a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message exchanged with a CLI.
type Message struct {
	Role    Role
	Content string
}

// Request is the unified internal request handed to an adapter.
// It is deliberately NOT the OpenAI wire type so future features
// (structured content, tool calls, streaming) can evolve without leaking
// HTTP structs into adapters.
type Request struct {
	Model    string
	Messages []Message
}

// Result is the unified text response from an adapter.
type Result struct {
	Content string
}

// Invocation describes one CLI subprocess execution.
type Invocation struct {
	Exec  string   // absolute path to the executable (resolved by discovery)
	Args  []string // arguments, no shell involved
	Stdin string   // optional stdin payload
	Env   []string // extra KEY=VALUE environment entries

	// Parse extracts the assistant text from captured output.
	Parse func(stdout, stderr []byte) (Result, error)
}

// Runner executes a provider invocation. The process runner implements it;
// tests substitute a fake.
type Runner interface {
	Run(ctx context.Context, inv Invocation) (Result, error)
}

// Adapter turns a unified Request into a CLI Invocation.
type Adapter interface {
	Alias() string
	Name() string
	DisplayName() string
	Invoke(req Request) (Invocation, error)
}

// ErrPromptTooLong hints that a message sequence cannot be passed safely on
// the Windows command line.
var ErrPromptTooLong = fmt.Errorf("prompt too long for command line")

// Error is a normalized, provider-scoped run error. The process runner and
// queue layer surface failures through provider.Error so the API layer can
// map them without guessing.
type Error struct {
	Code     string // normalized code, e.g. "provider_rate_limited"
	Provider string // provider alias
	Message  string // user-friendly message (never raw stderr as primary)
	Status   int    // suggested HTTP status
	Details  string // sanitized technical detail, shown only in GUI/debug
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// WithDetails attaches sanitized technical detail (GUI/debug only).
func (e *Error) WithDetails(d string) *Error {
	e.Details = d
	return e
}

// AsError unpacks a *Error from err. Accepts both a *Error pointer and a
// value-form Error (queue/proc return the dereferenced value).
func AsError(err error, target **Error) bool {
	if e, ok := err.(*Error); ok {
		*target = e
		return true
	}
	if e, ok := err.(Error); ok {
		*target = &e
		return true
	}
	return false
}

// Normalized error codes exposed to clients.
const (
	ErrProviderDisabled      = "provider_disabled"
	ErrProviderUnavailable   = "provider_unavailable"
	ErrProviderAuth          = "provider_authentication_error"
	ErrProviderRateLimited   = "provider_rate_limited"
	ErrProviderBusy          = "provider_busy"
	ErrQueueTimeout          = "queue_timeout"
	ErrProviderTimeout       = "provider_timeout"
	ErrProviderProcess       = "provider_process_error"
	ErrModelNotFound         = "model_not_found"
	ErrInvalidRequest        = "invalid_request_error"
	ErrStreamingNotSupported = "streaming_not_supported"
	ErrRequestCancelled      = "request_cancelled"
)

// NewError builds a normalized provider error.
func NewError(code, provider, message string, status int) *Error {
	return &Error{Code: code, Provider: provider, Message: message, Status: status}
}
