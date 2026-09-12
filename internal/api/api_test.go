package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LocalAIProxy/internal/activity"
	"LocalAIProxy/internal/provider"
)

type fakeModel struct {
	enabled    bool
	provider   string
	streamMode string
}

type fakeBackend struct {
	aliases    []string
	port       int
	host       string
	running    bool
	requireKey bool
	validKey   string
	info       map[string]ProviderInfo
	models     map[string]fakeModel
	runResult  provider.Result
	runErr     *provider.Error
	calledWith []string
}

func (f *fakeBackend) Host() string {
	if f.host == "" {
		return "127.0.0.1"
	}
	return f.host
}
func (f *fakeBackend) ModelCheck(id string) ModelCheck {
	m, ok := f.models[id]
	if !ok {
		return ModelCheck{}
	}
	return ModelCheck{Exists: true, Enabled: m.enabled, Provider: m.provider}
}
func (f *fakeBackend) EnabledModels() []string {
	out := make([]string, 0, len(f.models))
	for id, m := range f.models {
		if m.enabled {
			out = append(out, id)
		}
	}
	return out
}
func (f *fakeBackend) ProviderInfo(alias string) ProviderInfo {
	return f.info[alias]
}
func (f *fakeBackend) Aliases() []string         { return f.aliases }
func (f *fakeBackend) Port() int                 { return f.port }
func (f *fakeBackend) IsRunning() bool           { return f.running }
func (f *fakeBackend) GlobalConcurrency() int    { return 1 }
func (f *fakeBackend) RequiresAPIKey() bool      { return f.requireKey }
func (f *fakeBackend) ValidAPIKey(t string) bool { return t == f.validKey && t != "" }
func (f *fakeBackend) RunChat(ctx context.Context, req provider.Request) (provider.Result, *provider.Error) {
	f.calledWith = append(f.calledWith, req.Model)
	return f.runResult, f.runErr
}
func (f *fakeBackend) RunChatStream(ctx context.Context, req provider.Request, emit func(provider.StreamEvent)) (provider.Result, *provider.Error) {
	m := f.models[req.Model]
	if m.streamMode != "native" {
		return provider.Result{}, provider.NewError(provider.ErrStreamingNotSupported, req.Model, "This model does not allow streaming.", 400)
	}
	f.calledWith = append(f.calledWith, req.Model)
	emit(provider.StreamEvent{Text: "part1 "})
	emit(provider.StreamEvent{Text: "part2"})
	return provider.Result{Content: "part1 part2"}, nil
}

func newServer(fb *fakeBackend) *Server {
	s := New(fb, activity.New(20), nil)
	ctx, cancel := context.WithCancel(context.Background())
	s.rootCtx = ctx
	s.stopped = cancel
	return s
}

func doReq(s *Server, method, path, body, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Content-Type", "application/json")
	// reuse the real mux + middleware like Start does
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/v1/completions", s.handleChat)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	h := s.withCORS(s.withAuth(mux))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func defaultBackend() *fakeBackend {
	return &fakeBackend{
		aliases: []string{"claude", "codex", "gemini", "opencode"},
		port:    8317,
		running: true,
		info: map[string]ProviderInfo{
			"claude":   {Alias: "claude", Name: "Claude Code", Enabled: true, Installed: true, Ready: true},
			"codex":    {Alias: "codex", Name: "Codex", Enabled: true, Installed: true, Ready: true},
			"gemini":   {Alias: "gemini", Name: "Gemini CLI", Enabled: true, Installed: true, Ready: true},
			"opencode": {Alias: "opencode", Name: "OpenCode", Enabled: true, Installed: true, Ready: true},
		},
		models: map[string]fakeModel{
			"claude":   {enabled: true, provider: "claude", streamMode: "native"},
			"codex":    {enabled: true, provider: "codex", streamMode: "disabled"},
			"gemini":   {enabled: true, provider: "gemini", streamMode: "disabled"},
			"opencode": {enabled: true, provider: "opencode", streamMode: "native"},
		},
		runResult: provider.Result{Content: "hello from the CLI"},
	}
}

func TestModelsListsAllAliases(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "GET", "/v1/models", "", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 4 {
		t.Fatalf("models = %d", len(body.Data))
	}
	got := map[string]bool{}
	for _, m := range body.Data {
		got[m.ID] = true
	}
	for _, want := range []string{"claude", "codex", "gemini", "opencode"} {
		if !got[want] {
			t.Fatalf("missing model %q in %v", want, got)
		}
	}
}

func TestChatSuccess(t *testing.T) {
	fb := defaultBackend()
	s := newServer(fb)
	body := `{"model":"codex","messages":[{"role":"user","content":"hi"}]}`
	rr := doReq(s, "POST", "/v1/chat/completions", body, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "codex" || len(resp.Choices) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Choices[0].Message.Content != "hello from the CLI" {
		t.Fatalf("content = %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].Message.Role != "assistant" || resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("choice = %+v", resp.Choices[0])
	}
}

func TestUnknownModel(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"grok","messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "model_not_found")
}

func TestStreamDisabledModelErrorsViaSSE(t *testing.T) {
	// stream=true on a model whose stream_mode is disabled: HTTP 200 (headers
	// already sent) with an SSE error event, then [DONE] — never a silent
	// one-shot JSON conversion.
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"codex","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"type":"streaming_not_supported"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "data: [DONE]") {
		t.Fatalf("missing [DONE]: %s", rr.Body.String())
	}
}

func TestStreamNativeSSE(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("no chunk objects: %s", body)
	}
	if !strings.Contains(body, "part1 ") || !strings.Contains(body, "part2") {
		t.Fatalf("missing delta text: %s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("missing final chunk: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]\n") {
		t.Fatalf("missing [DONE]: %s", body)
	}
}

func TestBadJSON(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_request_error")
}

func TestMissingModel(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_request_error")
}

func TestEmptyMessages(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[]}`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_request_error")
}

func TestBadRole(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"assistantx","content":"x"}]}`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_request_error")
}

func TestMethodNotAllowed(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "GET", "/v1/chat/completions", "", "")
	if rr.Code != 405 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_request_error")
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		code   string
		status int
	}{
		{provider.ErrProviderDisabled, 400},
		{provider.ErrProviderBusy, 429},
		{provider.ErrQueueTimeout, 504},
		{provider.ErrProviderAuth, 401},
	}
	for _, tc := range cases {
		fb := defaultBackend()
		fb.runErr = provider.NewError(tc.code, "claude", "something happened", tc.status)
		s := newServer(fb)
		rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "")
		if rr.Code != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.code, rr.Code, tc.status)
		}
		assertErrorCode(t, rr, tc.code)
	}
}

func TestAuthRequired(t *testing.T) {
	fb := defaultBackend()
	fb.requireKey = true
	fb.validKey = "goodkey"
	s := newServer(fb)

	// missing -> 401
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 401 {
		t.Fatalf("missing key status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "provider_authentication_error")

	// wrong -> 401
	rr = doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "Bearer wrong")
	if rr.Code != 401 {
		t.Fatalf("wrong key status = %d", rr.Code)
	}

	// right -> 200
	rr = doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "Bearer goodkey")
	if rr.Code != 200 {
		t.Fatalf("good key status = %d", rr.Code)
	}
}

func TestAuthNotRequired(t *testing.T) {
	fb := defaultBackend()
	fb.requireKey = false
	s := newServer(fb)
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	s := newServer(defaultBackend())
	req := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	s.withCORS(s.withAuth(http.NewServeMux())).ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Fatalf("status = %d", rr.Code)
	}
	// Local-first: no permissive wildcard CORS by default.
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v == "*" {
		t.Fatalf("permissive CORS wildcard must not be set by default")
	}
}

func TestHealth(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "GET", "/health", "", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body = %s", rr.Body.String())
	}
}

func assertErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad error body %s: %v", rr.Body.String(), err)
	}
	if body.Error.Type != want {
		t.Fatalf("error.type = %q, want %q (body %s)", body.Error.Type, want, rr.Body.String())
	}
}
