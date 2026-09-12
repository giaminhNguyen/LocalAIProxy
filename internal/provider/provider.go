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
	// ContentParts carries richer OpenAI content (text/image/tool parts).
	// Adapters flatten to a string at the CLI boundary when needed.
	ContentParts []ContentPart `json:"-"`
	// ToolCalls / ToolCallID carry agent tool semantics through the core.
	ToolCalls  []ToolCall `json:"-"`
	ToolCallID string     `json:"-"`
	Name       string     `json:"-"`
}

// ContentPart is one extensible message part.
type ContentPart struct {
	Type     string `json:"type"` // text | image_url | input_text ...
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// ToolCall is one assistant-requested tool invocation.
type ToolCall struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition is one client-declared tool.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ResponseFormat captures structured-output requests.
type ResponseFormat struct {
	Type       string `json:"type"` // json_object | json_schema | text
	SchemaName string `json:"schema_name,omitempty"`
	Schema     any    `json:"schema,omitempty"`
}

// Request is the unified internal request handed to an adapter.
// It is deliberately NOT the OpenAI wire type so future features
// (structured content, tool calls, streaming) can evolve without leaking
// HTTP structs into adapters.
type Request struct {
	Model    string
	Messages []Message
	// Agent/OpenAI extensions — adapters flatten or reject honestly.
	Tools          []ToolDefinition `json:"-"`
	ToolChoice     string           `json:"-"`
	ResponseFormat *ResponseFormat  `json:"-"`
	Temperature    *float64         `json:"-"`
	MaxTokens      *int             `json:"-"`
	ContextWindow  *int             `json:"-"`
	StreamOptions  *StreamOptions   `json:"-"`
	UpstreamModel  string           `json:"-"`
	SystemPrompt   string           `json:"-"`
	ExtraArgs      []string         `json:"-"`
	WorkingDir     string           `json:"-"`
}

// StreamOptions mirrors OpenAI stream_options (e.g. include_usage).
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Result is the unified text response from an adapter.
type Result struct {
	Content string
	// Usage is only populated when the CLI truly reports it. Never fabricated.
	Usage *Usage `json:"-"`
	// ToolCalls is populated when the backend natively returns tool calls.
	ToolCalls []ToolCall `json:"-"`
	// FinishReason distinguishes stop / error / cancelled / timeout.
	FinishReason string `json:"-"`
}

// Usage holds real token counts when available.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamEvent is one normalized incremental event mid-stream.
// Text is kept for backward compatibility; new code should use Type +
// ContentDelta / FinishReason / Error / Usage / ToolCall.
type StreamEvent struct {
	Text string
	// Normalized fields:
	Type         string    `json:"-"`
	ContentDelta string    `json:"-"`
	ToolCall     *ToolCall `json:"-"`
	Usage        *Usage    `json:"-"`
	FinishReason string    `json:"-"`
	Err          error     `json:"-"`
}

// Stream event types.
const (
	StreamContentDelta = "content_delta"
	StreamToolCall     = "tool_call"
	StreamUsage        = "usage"
	StreamFinish       = "finish"
	StreamError        = "error"
)

// Capabilities describes what a provider backend can honestly do.
// Capabilities belong to Provider implementations; a ModelProfile applies
// policy on top (e.g. stream_mode=native only when Streaming=true).
type Capabilities struct {
	Streaming        bool `json:"streaming"`
	Tools            bool `json:"tools"`
	StructuredOutput bool `json:"structured_output"`
	Usage            bool `json:"usage"`
	Vision           bool `json:"vision"`
	ModelSelection   bool `json:"model_selection"`
	Sessions         bool `json:"sessions"`
}

// TextDelta returns the text this event contributes (compat helper).
func (e StreamEvent) TextDelta() string {
	if e.ContentDelta != "" {
		return e.ContentDelta
	}
	return e.Text
}

// StreamParseLine parses one complete stdout line during streaming and
// returns the assistant text delta it contributed, whether the stream is
// finished, and any error. Lines with no text delta return "".
type StreamParseLine func(line string) (delta string, done bool, err error)

// Invocation describes one CLI subprocess execution.
type Invocation struct {
	Exec  string   // absolute path to the executable (resolved by discovery)
	Args  []string // arguments, no shell involved
	Stdin string   // optional stdin payload
	Env   []string // extra KEY=VALUE environment entries

	// Parse extracts the assistant text from captured output.
	Parse func(stdout, stderr []byte) (Result, error)

	// StreamParse extracts incremental text from stdout lines. When set the
	// invocation is run in streaming mode (see Runner.RunStream).
	StreamParse StreamParseLine
}

// Runner executes a provider invocation. The process runner implements it;
// tests substitute a fake.
type Runner interface {
	Run(ctx context.Context, inv Invocation) (Result, error)
	RunStream(ctx context.Context, inv Invocation, emit func(StreamEvent)) (Result, error)
}

// Adapter turns a unified Request into a CLI Invocation.
type Adapter interface {
	Alias() string
	Name() string
	DisplayName() string
	Capabilities() Capabilities
	Invoke(req Request) (Invocation, error)
	// StreamInvoke builds an Invocation with StreamParse set, used when the
	// model profile allows native streaming. Adapters return an invocation
	// whose StreamParse is nil if the backend cannot stream coherently.
	StreamInvoke(req Request) (Invocation, error)
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
	ErrUnsupportedFeature    = "unsupported_feature"
)

// IsAuthError reports auth failures that must never be retried automatically.
func IsAuthError(code string) bool {
	return code == ErrProviderAuth
}

// IsQuotaError reports quota/rate failures that must never be retried aggressively.
func IsQuotaError(code string) bool {
	return code == ErrProviderRateLimited
}

// IsNonRetryable reports errors where automatic retry is forbidden:
// auth, quota, account restrictions, client cancellation.
func IsNonRetryable(code string) bool {
	switch code {
	case ErrProviderAuth, ErrProviderRateLimited, ErrRequestCancelled,
		ErrModelNotFound, ErrInvalidRequest, ErrProviderDisabled,
		ErrStreamingNotSupported, ErrUnsupportedFeature:
		return true
	}
	return false
}

// NewError builds a normalized provider error.
func NewError(code, provider, message string, status int) *Error {
	return &Error{Code: code, Provider: provider, Message: message, Status: status}
}

// rawTextStreamParser forwards stdout lines verbatim as text deltas. Used by
// backends that print plain text (Gemini CLI, OpenCode).
func rawTextStreamParser() StreamParseLine {
	return func(line string) (string, bool, error) {
		return line, false, nil
	}
}

// FlattenContent returns the plain-text prompt for a message, joining
// ContentParts when Content is empty. Adapters call this at the CLI boundary.
func FlattenContent(m Message) string {
	if m.Content != "" {
		return m.Content
	}
	var sb string
	for _, p := range m.ContentParts {
		if p.Type == "" || p.Type == "text" || p.Type == "input_text" {
			if p.Text != "" {
				if sb != "" {
					sb += "\n"
				}
				sb += p.Text
			}
		}
	}
	return sb
}

// FlattenMessages joins all messages via the legacy [role] format.
func FlattenMessages(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Content == "" && len(m.ContentParts) > 0 {
			m.Content = FlattenContent(m)
		}
		out = append(out, m)
	}
	return out
}
