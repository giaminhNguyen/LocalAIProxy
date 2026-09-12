// Package api implements the local OpenAI-compatible HTTP API bound to
// 127.0.0.1 only. Client-facing errors are normalized; raw stderr is never
// the primary message and machine-local diagnostics stay out of responses.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"LocalAIProxy/internal/activity"
	"LocalAIProxy/internal/logr"
	"LocalAIProxy/internal/provider"
)

// ProviderInfo is a read-only view of one provider's runtime state.
type ProviderInfo struct {
	Alias          string
	Name           string
	Enabled        bool
	Installed      bool
	Version        string
	Authentication string // detected | unknown | required
	Ready          bool
}

// ModelCheck is the backends answer to "does this model exist and can it be used".
type ModelCheck struct {
	Exists   bool
	Enabled  bool
	Provider string // provider alias the model routes to
}

// Backend is the core API surface the HTTP server needs. Implemented by core.
type Backend interface {
	// Models
	ModelCheck(id string) ModelCheck
	EnabledModels() []string

	// Providers
	ProviderInfo(alias string) ProviderInfo
	Aliases() []string

	// Server
	Host() string
	Port() int
	IsRunning() bool
	GlobalConcurrency() int

	// Chat
	RunChat(ctx context.Context, req provider.Request) (provider.Result, *provider.Error)
	RunChatStream(ctx context.Context, req provider.Request, emit func(provider.StreamEvent)) (provider.Result, *provider.Error)

	// Auth
	RequiresAPIKey() bool
	ValidAPIKey(bearerToken string) bool
}

// Server is the HTTP front end.
type Server struct {
	backend  Backend
	activity *activity.Log
	logger   *logr.Logger

	httpSrv *http.Server
	stopped context.CancelFunc
	rootCtx context.Context
}

// New wires the HTTP server to a backend.
func New(backend Backend, act *activity.Log, logger *logr.Logger) *Server {
	return &Server{backend: backend, activity: act, logger: logger}
}

// Start binds <host>:<port> and serves. Returns an error if the address is
// already in use (the caller surfaces it — we never silently change port).
func (s *Server) Start() error {
	if s.httpSrv != nil {
		return errors.New("server already running")
	}
	rootCtx, stop := context.WithCancel(context.Background())
	s.rootCtx = rootCtx
	s.stopped = stop

	addr := fmt.Sprintf("%s:%d", s.backend.Host(), s.backend.Port())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.stopped()
		return fmt.Errorf("address in use: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/v1/completions", s.handleChat)
	mux.HandleFunc("/v1/responses", s.handleResponses)

	s.httpSrv = &http.Server{
		Handler:           s.withCORS(s.withAuth(mux)),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	return nil
}

// Stop cancels all in-flight proxy-owned requests (killing their CLI children)
// and shuts the listener down. Safe to call when not running.
func (s *Server) Stop() {
	if s.stopped != nil {
		s.stopped()
		s.stopped = nil
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.httpSrv.Shutdown(ctx)
		cancel()
		s.httpSrv = nil
	}
}

// Addr returns the bound address ("<host>:<port>").
func (s *Server) Addr() string { return fmt.Sprintf("%s:%d", s.backend.Host(), s.backend.Port()) }

// RequestCtx returns a per-request context cancelled by client disconnect,
// server shutdown, or handler return (whichever comes first). The watcher
// goroutine exits on any of those, so it never leaks.
func (s *Server) RequestCtx(r *http.Request) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(s.rootCtx)
	go func() {
		select {
		case <-r.Context().Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.backend.RequiresAPIKey() {
			next.ServeHTTP(w, r)
			return
		}
		token := tokenAfterBearer(r.Header.Get("Authorization"))
		if !s.backend.ValidAPIKey(token) {
			writeError(w, 401, provider.NewError(provider.ErrProviderAuth, "", "Invalid or missing API key. Set Authorization: Bearer <key>.", 401))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokenAfterBearer(auth string) string {
	const p = "bearer "
	if len(auth) > len(p) && strings.EqualFold(auth[:len(p)], p) {
		return strings.TrimSpace(auth[len(p):])
	}
	return ""
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Local-first: no permissive Access-Control-Allow-Origin by default.
		// Browser clients on the same loopback origin don't need a wildcard;
		// CLI/desktop SDKs don't use CORS at all. Only answer preflights
		// minimally without advertising cross-origin access.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- handlers -------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	providers := make(map[string]any)
	for _, alias := range s.backend.Aliases() {
		p := s.backend.ProviderInfo(alias)
		providers[alias] = map[string]any{
			"enabled":        p.Enabled,
			"installed":      p.Installed,
			"authentication": p.Authentication,
			"ready":          p.Ready,
			"name":           p.Name,
			"version":        p.Version,
		}
	}
	status := "ok"
	if !s.backend.IsRunning() {
		status = "stopped"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"server": map[string]any{
			"running": s.backend.IsRunning(),
			"host":    s.backend.Host(),
			"port":    s.backend.Port(),
		},
		"providers": providers,
	})
}

// handleModels lists every ENABLED model profile. Disabled profiles are
// hidden, matching the prompt requirement that only usable models are advertised.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := make([]map[string]any, 0, 8)
	for _, id := range s.backend.EnabledModels() {
		data = append(data, map[string]any{
			"id":       id,
			"object":   "model",
			"owned_by": "local-ai-proxy",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// chatRequest is the accepted OpenAI /v1/chat/completions subset, extended for
// agent compatibility: tools, tool_choice, response_format, stream_options,
// richer message content (string or parts), and tool result messages.
type chatRequest struct {
	Model          string           `json:"model"`
	Messages       []chatMessageIn  `json:"messages"`
	Stream         bool             `json:"stream"`
	Tools          []toolIn         `json:"tools"`
	ToolChoice     any              `json:"tool_choice"`
	ResponseFormat *respFormatIn    `json:"response_format"`
	StreamOptions  *streamOptsIn    `json:"stream_options"`
	Temperature    *float64         `json:"temperature"`
	MaxTokens      *int             `json:"max_tokens"`
	MaxCompletionTokens *int        `json:"max_completion_tokens"`
}

type toolIn struct {
	Type     string         `json:"type"`
	Function toolFuncIn     `json:"function"`
}

type toolFuncIn struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type respFormatIn struct {
	Type       string         `json:"type"`
	JSONSchema map[string]any `json:"json_schema"`
}

type streamOptsIn struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessageIn struct {
	Role       string `json:"role"`
	Content    any    `json:"content"`
	ToolCalls  []toolCallIn `json:"tool_calls"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
}

type toolCallIn struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function toolCallFuncIn  `json:"function"`
}

type toolCallFuncIn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, provider.NewError(provider.ErrInvalidRequest, "", "Method not allowed. Use POST.", 405))
		return
	}
	reqCtx, cancel := s.RequestCtx(r)
	defer cancel()

	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	var cr chatRequest
	if err := dec.Decode(&cr); err != nil {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, "", "Invalid JSON request body.", 400))
		return
	}

	if cr.Model == "" {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, "", "Missing required field: model.", 400))
		return
	}
	check := s.backend.ModelCheck(cr.Model)
	if !check.Exists {
		msg := fmt.Sprintf("Unknown model %q.", cr.Model)
		if en := s.backend.EnabledModels(); len(en) > 0 {
			msg += " Use one of: " + strings.Join(en, ", ") + "."
		}
		writeError(w, 400, provider.NewError(provider.ErrModelNotFound, cr.Model, msg, 400))
		return
	}
	if !check.Enabled {
		writeError(w, 400, provider.NewError(provider.ErrProviderDisabled, cr.Model, fmt.Sprintf("Model %q is disabled in LocalAIProxy settings.", cr.Model), 400))
		return
	}
	if len(cr.Messages) == 0 {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, cr.Model, "messages must not be empty.", 400))
		return
	}

	req, perr := toProviderRequest(cr)
	if perr != nil {
		writeError(w, perr.Status, perr)
		s.recordActivity(cr.Model, check.Provider, 0, cr.Stream, 0, 0, perr)
		return
	}
	// Honest tool handling: CLI backends don't support OpenAI tools.
	// Never silently ignore tool requests — fail with a clear error.
	if len(req.Tools) > 0 {
		pe := provider.NewError(provider.ErrUnsupportedFeature, cr.Model, "This backend does not support tool calling. Remove tools/tool_choice from the request.", 400)
		writeError(w, 400, pe)
		s.recordActivity(cr.Model, check.Provider, 0, cr.Stream, 0, 0, pe)
		return
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type != "" && req.ResponseFormat.Type != "text" {
		// Structured output is not natively enforced by CLIs; pass through as
		// instruction rather than pretending schema validation.
		// We still accept it so clients aren't blocked, but don't claim validation.
	}

	start := time.Now()
	queueStart := start
	var (
		result provider.Result
	)
	if cr.Stream {
		res, streamErr, ttft := s.handleChatStream(w, r, reqCtx, cr, req)
		dur := time.Since(start).Milliseconds()
		s.recordActivity(cr.Model, check.Provider, dur, true, time.Since(queueStart).Milliseconds(), ttft, streamErr)
		if streamErr != nil {
			// SSE handler already wrote the error event.
			_ = res
			return
		}
		return // SSE stream already finished with [DONE]
	}
	result, perr = s.backend.RunChat(reqCtx, req)
	dur := time.Since(start).Milliseconds()

	s.recordActivity(cr.Model, check.Provider, dur, false, dur, 0, perr)

	if perr != nil {
		writeError(w, perr.Status, perr)
		return
	}

	msg := map[string]any{"role": "assistant", "content": result.Content}
	if len(result.ToolCalls) > 0 {
		tcs := make([]any, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			tcs = append(tcs, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
		}
		msg["tool_calls"] = tcs
	}
	resp := map[string]any{
		"id":      chatID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   cr.Model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": msg,
				"finish_reason": finishReason(result, perr),
			},
		},
		// No fabricated usage numbers.
	}
	if result.Usage != nil {
		resp["usage"] = map[string]any{"prompt_tokens": result.Usage.PromptTokens, "completion_tokens": result.Usage.CompletionTokens, "total_tokens": result.Usage.TotalTokens}
	}
	writeJSON(w, http.StatusOK, resp)
}

func finishReason(res provider.Result, perr *provider.Error) string {
	if perr != nil {
		return "error"
	}
	if res.FinishReason != "" {
		return res.FinishReason
	}
	return "stop"
}

// toProviderRequest maps the OpenAI wire format into the shared canonical core.
func toProviderRequest(cr chatRequest) (provider.Request, *provider.Error) {
	req := provider.Request{Model: cr.Model}
	if cr.Temperature != nil {
		req.Temperature = cr.Temperature
	}
	if cr.MaxTokens != nil {
		req.MaxTokens = cr.MaxTokens
	} else if cr.MaxCompletionTokens != nil {
		req.MaxTokens = cr.MaxCompletionTokens
	}
	if cr.StreamOptions != nil {
		req.StreamOptions = &provider.StreamOptions{IncludeUsage: cr.StreamOptions.IncludeUsage}
	}
	if cr.ResponseFormat != nil {
		rf := &provider.ResponseFormat{Type: cr.ResponseFormat.Type}
		if cr.ResponseFormat.JSONSchema != nil {
			if n, ok := cr.ResponseFormat.JSONSchema["name"].(string); ok {
				rf.SchemaName = n
			}
			rf.Schema = cr.ResponseFormat.JSONSchema["schema"]
		}
		req.ResponseFormat = rf
	}
	for _, t := range cr.Tools {
		name := t.Function.Name
		if name == "" {
			return provider.Request{}, provider.NewError(provider.ErrInvalidRequest, cr.Model, "Each tool must have function.name.", 400)
		}
		req.Tools = append(req.Tools, provider.ToolDefinition{Name: name, Description: t.Function.Description, Parameters: t.Function.Parameters})
	}
	if cr.ToolChoice != nil {
		switch v := cr.ToolChoice.(type) {
		case string:
			req.ToolChoice = v
		default:
			b, _ := json.Marshal(v)
			req.ToolChoice = string(b)
		}
	}
	for _, m := range cr.Messages {
		role := provider.Role(m.Role)
		switch role {
		case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant:
		case "tool", "function", "developer":
			// Map tool/function results to user text at the CLI boundary.
			role = provider.RoleUser
		default:
			return provider.Request{}, provider.NewError(provider.ErrInvalidRequest, cr.Model, "Unsupported message role. Use system, user or assistant.", 400)
		}
		pm := provider.Message{Role: role, ToolCallID: m.ToolCallID, Name: m.Name}
		switch c := m.Content.(type) {
		case nil:
			pm.Content = ""
		case string:
			pm.Content = c
		case []any:
			for _, part := range c {
				pm2, ok := part.(map[string]any)
				if !ok {
					continue
				}
				pt, _ := pm2["type"].(string)
				if pt == "text" || pt == "input_text" {
					if tx, ok := pm2["text"].(string); ok {
						pm.ContentParts = append(pm.ContentParts, provider.ContentPart{Type: "text", Text: tx})
					} else if txm, ok := pm2["text"].(map[string]any); ok {
						if v, ok := txm["value"].(string); ok {
							pm.ContentParts = append(pm.ContentParts, provider.ContentPart{Type: "text", Text: v})
						}
					}
				} else if pt == "image_url" {
					// Vision unsupported by CLIs — record honestly, flatten skips it.
					pm.ContentParts = append(pm.ContentParts, provider.ContentPart{Type: "image_url"})
				} else if tx, ok := pm2["text"].(string); ok {
					pm.ContentParts = append(pm.ContentParts, provider.ContentPart{Type: pt, Text: tx})
				}
			}
			pm.Content = provider.FlattenContent(pm)
		default:
			b, _ := json.Marshal(c)
			pm.Content = string(b)
		}
		for _, tc := range m.ToolCalls {
			pm.ToolCalls = append(pm.ToolCalls, provider.ToolCall{ID: tc.ID, Type: tc.Type, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
		}
		req.Messages = append(req.Messages, pm)
	}
	return req, nil
}

// handleChatStream emits OpenAI-compatible SSE chunks while the CLI runs.
// A failed stream emits an SSE error event with finish_reason=error (never
// stop) and returns the error so activity records failure/cancelled/timeout
// instead of Success.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, reqCtx context.Context, cr chatRequest, req provider.Request) (provider.Result, *provider.Error, int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return provider.Result{}, provider.NewError(provider.ErrInvalidRequest, cr.Model, "Streaming unsupported on this connection.", 400), 0
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	streamID := chatID()
	start := time.Now()
	var ttft int64 = -1
	var sb strings.Builder
	emit := func(ev provider.StreamEvent) {
		delta := ev.TextDelta()
		if delta == "" {
			return
		}
		if ttft < 0 {
			ttft = time.Since(start).Milliseconds()
		}
		sb.WriteString(delta)
		chunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   cr.Model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{"role": "assistant", "content": delta},
					"finish_reason": nil,
				},
			},
		}
		writeSSE(w, flusher, chunk)
	}

	result, perr := s.backend.RunChatStream(reqCtx, req, emit)

	// Backend produced content but never emitted (parser edge case): emit it.
	if result.Content != "" && sb.Len() == 0 {
		emit(provider.StreamEvent{Text: result.Content, ContentDelta: result.Content})
	}

	if perr != nil {
		body := map[string]any{
			"error": map[string]any{
				"type":     perr.Code,
				"provider": perr.Provider,
				"message":  perr.Message,
			},
		}
		writeSSE(w, flusher, body)
		writeSSE(w, flusher, map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   cr.Model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": "error",
				},
			},
		})
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		if ttft < 0 {
			ttft = 0
		}
		return result, perr, ttft
	}

	usageChunk := map[string]any{}
	if result.Usage != nil && cr.StreamOptions != nil && cr.StreamOptions.IncludeUsage {
		usageChunk["usage"] = map[string]any{"prompt_tokens": result.Usage.PromptTokens, "completion_tokens": result.Usage.CompletionTokens, "total_tokens": result.Usage.TotalTokens}
	}
	finalChunk := map[string]any{
		"id":      streamID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   cr.Model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	}
	for k, v := range usageChunk {
		finalChunk[k] = v
	}
	writeSSE(w, flusher, finalChunk)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	if ttft < 0 {
		ttft = time.Since(start).Milliseconds()
	}
	return result, nil, ttft
}

// handleResponses implements POST /v1/responses on top of the same canonical
// core as chat completions (no second protocol stack).
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, provider.NewError(provider.ErrInvalidRequest, "", "Method not allowed. Use POST.", 405))
		return
	}
	reqCtx, cancel := s.RequestCtx(r)
	defer cancel()
	defer r.Body.Close()
	var in struct {
		Model       string          `json:"model"`
		Input       any             `json:"input"`
		Instructions string        `json:"instructions"`
		Stream      bool            `json:"stream"`
		Temperature *float64        `json:"temperature"`
		MaxTokens   *int            `json:"max_output_tokens"`
		Tools       []toolIn        `json:"tools"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	if err := dec.Decode(&in); err != nil {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, "", "Invalid JSON request body.", 400))
		return
	}
	if in.Model == "" {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, "", "Missing required field: model.", 400))
		return
	}
	check := s.backend.ModelCheck(in.Model)
	if !check.Exists {
		writeError(w, 400, provider.NewError(provider.ErrModelNotFound, in.Model, fmt.Sprintf("Unknown model %q.", in.Model), 400))
		return
	}
	if !check.Enabled {
		writeError(w, 400, provider.NewError(provider.ErrProviderDisabled, in.Model, fmt.Sprintf("Model %q is disabled.", in.Model), 400))
		return
	}
	// Map Responses input -> canonical messages.
	var msgs []provider.Message
	if in.Instructions != "" {
		msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: in.Instructions})
	}
	switch v := in.Input.(type) {
	case nil:
	case string:
		if strings.TrimSpace(v) != "" {
			msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: v})
		}
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role == "" {
				role = "user"
			}
			var content string
			if c, ok := m["content"].(string); ok {
				content = c
			} else if arr, ok := m["content"].([]any); ok {
				for _, p := range arr {
					pm, ok := p.(map[string]any)
					if !ok {
						continue
					}
					if tx, ok := pm["text"].(string); ok {
						content += tx
					}
				}
			}
			msgs = append(msgs, provider.Message{Role: provider.Role(role), Content: content})
		}
	default:
		b, _ := json.Marshal(v)
		msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: string(b)})
	}
	if len(msgs) == 0 {
		writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, in.Model, "input must not be empty.", 400))
		return
	}
	req := provider.Request{Model: in.Model, Messages: msgs, Temperature: in.Temperature, MaxTokens: in.MaxTokens}
	for _, t := range in.Tools {
		req.Tools = append(req.Tools, provider.ToolDefinition{Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters})
	}
	if len(req.Tools) > 0 {
		pe := provider.NewError(provider.ErrUnsupportedFeature, in.Model, "This backend does not support tool calling.", 400)
		writeError(w, 400, pe)
		return
	}
	start := time.Now()
	res, perr := s.backend.RunChat(reqCtx, req)
	dur := time.Since(start).Milliseconds()
	s.recordActivity(in.Model, check.Provider, dur, in.Stream, dur, 0, perr)
	if perr != nil {
		writeError(w, perr.Status, perr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     "resp-" + strings.TrimPrefix(chatID(), "chatcmpl-"),
		"object": "response",
		"model":  in.Model,
		"status": "completed",
		"output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": res.Content}}}},
	})
}

// recordActivity logs one chat outcome with honest status categories.
func (s *Server) recordActivity(model, alias string, dur int64, stream bool, queueWait, ttft int64, perr *provider.Error) {
	info := s.backend.ProviderInfo(alias)
	entry := activity.Entry{
		Time:        time.Now().Format("15:04:05"),
		Provider:    info.Name,
		Alias:       model,
		Model:       model,
		DurationMS:  dur,
		Stream:      stream,
		QueueWaitMS: queueWait,
		TTFTMS:      ttft,
	}
	if perr != nil {
		entry.Status = perr.Code
		entry.OK = false
		entry.ErrCategory = errCategory(perr.Code)
		if perr.Code == provider.ErrRequestCancelled {
			entry.CancelReason = "client disconnected"
		}
	} else {
		entry.Status = "Success"
		entry.OK = true
		entry.ErrCategory = "success"
	}
	s.activity.Add(entry)
}

func errCategory(code string) string {
	switch code {
	case provider.ErrRequestCancelled:
		return "cancelled"
	case provider.ErrProviderTimeout, provider.ErrQueueTimeout:
		return "timeout"
	case provider.ErrProviderBusy:
		return "queue_rejected"
	case provider.ErrProviderAuth:
		return "auth"
	case provider.ErrProviderRateLimited:
		return "quota"
	case "":
		return "success"
	default:
		return "failure"
	}
}

// writeSSE sends one SSE data event.
func writeSSE(w http.ResponseWriter, f http.Flusher, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}

// Error payload helpers ----------------------------------------------------

func writeError(w http.ResponseWriter, status int, e *provider.Error) {
	body := map[string]any{
		"error": map[string]any{
			"type":     e.Code,
			"provider": e.Provider,
			"message":  e.Message,
		},
	}
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func chatID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}
