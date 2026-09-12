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

type fakeBackend struct {
	aliases    []string
	port       int
	running    bool
	requireKey bool
	validKey   string
	info       map[string]ProviderInfo
	runResult  provider.Result
	runErr     *provider.Error
	calledWith []string
}

func (f *fakeBackend) ProviderInfo(alias string) ProviderInfo {
	return f.info[alias]
}
func (f *fakeBackend) Aliases() []string         { return f.aliases }
func (f *fakeBackend) Port() int                 { return f.port }
func (f *fakeBackend) IsRunning() bool           { return f.running }
func (f *fakeBackend) RequiresAPIKey() bool      { return f.requireKey }
func (f *fakeBackend) ValidAPIKey(t string) bool { return t == f.validKey && t != "" }
func (f *fakeBackend) RunChat(ctx context.Context, req provider.Request) (provider.Result, *provider.Error) {
	f.calledWith = append(f.calledWith, req.Model)
	return f.runResult, f.runErr
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
	for i, want := range []string{"claude", "codex", "gemini", "opencode"} {
		if body.Data[i].ID != want {
			t.Fatalf("models[%d] = %q", i, body.Data[i].ID)
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

func TestStreamRejected(t *testing.T) {
	s := newServer(defaultBackend())
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","stream":true,"messages":[]}`, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d", rr.Code)
	}
	assertErrorCode(t, rr, "streaming_not_supported")
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
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","messages":[{"role":"tool","content":"x"}]}`, "")
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
	if !strings.Contains(rr.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("missing CORS header: %v", rr.Header())
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
