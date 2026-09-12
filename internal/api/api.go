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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
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

// chatRequest is the accepted OpenAI /v1/chat/completions subset.
type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessageIn `json:"messages"`
	Stream   bool            `json:"stream"`
}

type chatMessageIn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

	req := provider.Request{Model: cr.Model}
	for _, m := range cr.Messages {
		role := provider.Role(m.Role)
		switch role {
		case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant:
		default:
			writeError(w, 400, provider.NewError(provider.ErrInvalidRequest, cr.Model, "Unsupported message role. Use system, user or assistant.", 400))
			return
		}
		req.Messages = append(req.Messages, provider.Message{Role: role, Content: m.Content})
	}

	start := time.Now()
	var (
		result provider.Result
		perr   *provider.Error
	)
	if cr.Stream {
		result, perr = s.handleChatStream(w, r, reqCtx, cr, req)
	} else {
		result, perr = s.backend.RunChat(reqCtx, req)
	}
	dur := time.Since(start).Milliseconds()

	s.recordActivity(cr.Model, check.Provider, dur, perr)

	if perr != nil {
		// SSE handler already wrote the error event; body replies get JSON 4xx.
		if !cr.Stream {
			writeError(w, perr.Status, perr)
		}
		return
	}

	if cr.Stream {
		return // SSE stream already finished with [DONE]
	}

	resp := map[string]any{
		"id":      chatID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   cr.Model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": result.Content,
				},
				"finish_reason": "stop",
			},
		},
		// No fabricated usage numbers.
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleChatStream emits OpenAI-compatible SSE chunks while the CLI runs.
// Failure surfaces as an SSE "error" event (HTTP 200 already sent) followed
// by [DONE], which is how OpenAI-compatible clients read stream errors.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, reqCtx context.Context, cr chatRequest, req provider.Request) (provider.Result, *provider.Error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return provider.Result{}, provider.NewError(provider.ErrInvalidRequest, cr.Model, "Streaming unsupported on this connection.", 400)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	streamID := chatID()
	var sb strings.Builder
	emit := func(ev provider.StreamEvent) {
		if ev.Text == "" {
			return
		}
		sb.WriteString(ev.Text)
		chunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   cr.Model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{"role": "assistant", "content": ev.Text},
					"finish_reason": nil,
				},
			},
		}
		writeSSE(w, flusher, chunk)
	}

	result, perr := s.backend.RunChatStream(reqCtx, req, emit)

	// Backend produced content but never emitted (parser edge case): emit it.
	if result.Content != "" && sb.Len() == 0 {
		emit(provider.StreamEvent{Text: result.Content})
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
	}

	writeSSE(w, flusher, map[string]any{
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
	})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	return result, nil
}

// recordActivity logs one chat outcome.
func (s *Server) recordActivity(model, alias string, dur int64, perr *provider.Error) {
	info := s.backend.ProviderInfo(alias)
	entry := activity.Entry{
		Time:       time.Now().Format("15:04:05"),
		Provider:   info.Name,
		Alias:      model,
		DurationMS: dur,
	}
	if perr != nil {
		entry.Status = perr.Code
		entry.OK = false
	} else {
		entry.Status = "Success"
		entry.OK = true
	}
	s.activity.Add(entry)
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
