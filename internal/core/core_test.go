package core

import (
	"context"
	"path/filepath"
	"testing"

	"LocalAIProxy/internal/activity"
	"LocalAIProxy/internal/config"
	"LocalAIProxy/internal/discovery"
	"LocalAIProxy/internal/provider"
	"LocalAIProxy/internal/queue"
)

type fakeRunner struct {
	calls   int
	called  []string
	result  provider.Result
	wantErr error
}

func (r *fakeRunner) Run(ctx context.Context, inv provider.Invocation) (provider.Result, error) {
	r.calls++
	r.called = append(r.called, inv.Exec)
	return r.result, r.wantErr
}

func (r *fakeRunner) RunStream(ctx context.Context, inv provider.Invocation, emit func(provider.StreamEvent)) (provider.Result, error) {
	r.calls++
	r.called = append(r.called, inv.Exec)
	if r.wantErr != nil {
		return provider.Result{}, r.wantErr
	}
	emit(provider.StreamEvent{Text: "tok1 "})
	emit(provider.StreamEvent{Text: "tok2"})
	return provider.Result{Content: "tok1 tok2"}, nil
}

// freshConfig returns defaults that persist to a throwaway temp file, so tests
// never touch the developer's real %APPDATA% config.
func freshConfig(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.LoadFile(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testCore(t *testing.T, cfg *config.Config, infos map[string]discovery.Info, runner *fakeRunner) *Core {
	t.Helper()
	return &Core{
		cfg:      cfg,
		infos:    infos,
		queues:   make(map[string]*queue.Queue),
		lastTest: make(map[string]*TestResult),
		runner:   runner,
		act:      activity.New(10),
	}
}

func defaultTestInfos() map[string]discovery.Info {
	return map[string]discovery.Info{
		"claude":   {Alias: "claude", Installed: false},
		"codex":    {Alias: "codex", Installed: true, Executable: "codex.exe", Version: "0.153", Auth: discovery.AuthDetected},
		"gemini":   {Alias: "gemini", Installed: true, Executable: "gemini.exe", Auth: discovery.AuthUnknown},
		"opencode": {Alias: "opencode", Installed: true, Executable: "opencode.exe", Auth: discovery.AuthRequired},
	}
}

func TestDisabledProviderRejected(t *testing.T) {
	cfg := freshConfig(t)
	p := cfg.Provider("codex")
	p.Enabled = false
	if err := cfg.SetProvider("codex", p); err != nil {
		t.Fatal(err)
	}

	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	_, perr := c.RunChat(context.Background(), provider.Request{Model: "codex"})
	if perr == nil {
		t.Fatal("expected error")
	}
	if perr.Code != provider.ErrProviderDisabled {
		t.Fatalf("code = %q", perr.Code)
	}
	if fk.calls != 0 {
		t.Fatal("runner should not be called for disabled provider")
	}
}

func TestNotInstalledProviderRejected(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	_, perr := c.RunChat(context.Background(), provider.Request{Model: "claude"})
	if perr == nil {
		t.Fatal("expected error")
	}
	if perr.Code != provider.ErrProviderUnavailable {
		t.Fatalf("code = %q", perr.Code)
	}
	if fk.calls != 0 {
		t.Fatal("runner should not be called for missing provider")
	}
}

func TestHappyPath(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{result: provider.Result{Content: "ok"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	res, perr := c.RunChat(context.Background(), provider.Request{Model: "codex", Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
	if perr != nil {
		t.Fatal(perr)
	}
	if res.Content != "ok" {
		t.Fatalf("content = %q", res.Content)
	}
	if len(fk.called) != 1 || fk.called[0] != "codex.exe" {
		t.Fatalf("called = %v", fk.called)
	}
}

func TestAuthRequiredStillRuns(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	// Auth required is surfaced in ProviderInfo/Snapshot, not a RunChat blocker.
	res, perr := c.RunChat(context.Background(), provider.Request{Model: "opencode", Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	if perr != nil {
		t.Fatal(perr)
	}
	if res.Content != "" {
		t.Fatalf("unexpected content %q", res.Content)
	}
}

func TestTestProviderSuccess(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{result: provider.Result{Content: "OK"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	tr := c.TestProvider(context.Background(), "codex")
	if !tr.Passed {
		t.Fatalf("test should pass: %+v", tr)
	}
	if tr.Response != "OK" {
		t.Fatalf("response = %q", tr.Response)
	}
	if c.LastTest("codex") == nil || !c.LastTest("codex").Passed {
		t.Fatal("LastTest not set")
	}
}

func TestTestProviderFailure(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{wantErr: provider.NewError(provider.ErrProviderAuth, "opencode", "login needed", 503)}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	tr := c.TestProvider(context.Background(), "opencode")
	if tr.Passed {
		t.Fatal("test should fail")
	}
	if tr.Message != "login needed" {
		t.Fatalf("message = %q", tr.Message)
	}
}

func TestSnapshotBuildsAllProviders(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})

	snap := c.Snapshot()
	if snap.Port != 8317 {
		t.Fatalf("port = %d", snap.Port)
	}
	if len(snap.Providers) != 4 {
		t.Fatalf("providers = %d", len(snap.Providers))
	}
	for _, p := range snap.Providers {
		switch p.Alias {
		case "claude":
			if p.Installed {
				t.Fatal("claude should show as not installed")
			}
			if p.Status != "Not installed" {
				t.Fatalf("claude status = %q", p.Status)
			}
		case "gemini":
			if p.Auth != "unknown" || p.Status != "Auth unknown" {
				t.Fatalf("gemini view = %+v", p)
			}
		case "opencode":
			if p.Auth != "required" || p.Status != "Auth required" {
				t.Fatalf("opencode view = %+v", p)
			}
		}
	}
}

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		view ProviderView
		want string
	}{
		{ProviderView{Installed: false}, "Not installed"},
		{ProviderView{Installed: true, Enabled: false}, "Disabled"},
		{ProviderView{Installed: true, Enabled: true, Auth: "required"}, "Auth required"},
		{ProviderView{Installed: true, Enabled: true, Auth: "unknown"}, "Auth unknown"},
		{ProviderView{Installed: true, Enabled: true, Auth: "detected"}, "Ready"},
		{ProviderView{Installed: true, Enabled: true, Auth: ""}, "Ready"},
	}
	for _, tc := range cases {
		status, kind := deriveStatus(tc.view)
		if status != tc.want {
			t.Errorf("view %+v: status = %q, want %q", tc.view, status, tc.want)
		}
		if tc.view.Auth == "unknown" && kind != "warn" {
			t.Errorf("unknown auth kind = %q", kind)
		}
	}
}

func TestSetPortAndConfig(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})

	if err := c.SetPort(9000); err != nil {
		t.Fatal(err)
	}
	if c.cfg.Server.Port != 9000 {
		t.Fatalf("port = %d", c.cfg.Server.Port)
	}
	if err := c.SetAutoStart(false); err != nil {
		t.Fatal(err)
	}
	if c.cfg.AutoStartServer {
		t.Fatal("autostart should be false")
	}
	if err := c.SetAPIKeyEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !c.cfg.RequireAPIKey {
		t.Fatal("requireApiKey should be true")
	}
	key, err := c.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) < 20 {
		t.Fatalf("key too short: %d", len(key))
	}
	if c.cfg.APIKey != key || c.CurrentAPIKey() != key {
		t.Fatal("key mismatch")
	}
	if !c.ValidAPIKey(key) || c.ValidAPIKey("wrong") || c.ValidAPIKey("") {
		t.Fatal("ValidAPIKey logic broken")
	}
}

func TestConfigureSettings(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})

	err := c.Configure(map[string]any{
		"port":            float64(7777),
		"autoStartServer": false,
		"providers": map[string]any{
			"codex": map[string]any{
				"enabled":         false,
				"concurrency":     float64(4),
				"maxQueue":        float64(20),
				"queueTimeoutSec": float64(60),
				"execTimeoutSec":  float64(300),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Server.Port != 7777 || c.cfg.AutoStartServer {
		t.Fatalf("port = %d, auto = %v", c.cfg.Server.Port, c.cfg.AutoStartServer)
	}
	p := c.cfg.Provider("codex")
	if p.Enabled || p.Concurrency != 4 || p.MaxQueue != 20 || p.QueueTimeoutSec != 60 || p.ExecTimeoutSec != 300 {
		t.Fatalf("codex config = %+v", p)
	}
}

func TestRestoreDefaults(t *testing.T) {
	cfg := freshConfig(t)
	cfg.Server.Port = 9999
	p := cfg.Provider("claude")
	p.Enabled = false
	if err := cfg.SetProvider("claude", p); err != nil {
		t.Fatal(err)
	}

	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	if err := c.RestoreDefaults(); err != nil {
		t.Fatal(err)
	}
	if c.cfg.Server.Port != 8317 {
		t.Fatalf("port = %d", c.cfg.Server.Port)
	}
	if !c.cfg.Provider("claude").Enabled {
		t.Fatal("claude should be enabled after restore")
	}
}

func TestFirstRunFlow(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	if !c.IsFirstRun() {
		t.Fatal("should be first run")
	}
	if err := c.DismissFirstRun(); err != nil {
		t.Fatal(err)
	}
	if c.IsFirstRun() {
		t.Fatal("first run should be dismissed")
	}
	if err := c.ResetFirstRun(); err != nil {
		t.Fatal(err)
	}
	if !c.IsFirstRun() {
		t.Fatal("first run should be reset")
	}
}

func TestActiveRequestsCount(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	if c.ActiveRequests() != 0 {
		t.Fatalf("active = %d", c.ActiveRequests())
	}
}

func TestActivitiesList(t *testing.T) {
	c := testCore(t, freshConfig(t), defaultTestInfos(), &fakeRunner{})
	if len(c.Activities()) != 0 {
		t.Fatal("expected empty activities")
	}
}

func TestUnknownModelRejected(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	_, perr := c.RunChat(context.Background(), provider.Request{Model: "grok-3"})
	if perr == nil {
		t.Fatal("expected error")
	}
	if perr.Code != provider.ErrModelNotFound {
		t.Fatalf("code = %q", perr.Code)
	}
	if fk.calls != 0 {
		t.Fatal("runner should not be called")
	}
}

func TestDisabledModelRejected(t *testing.T) {
	cfg := freshConfig(t)
	if err := cfg.SetModel("off-model", config.ModelProfile{Provider: "codex", Enabled: false, TimeoutSec: 300}); err != nil {
		t.Fatal(err)
	}
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	_, perr := c.RunChat(context.Background(), provider.Request{Model: "off-model"})
	if perr == nil || perr.Code != provider.ErrProviderDisabled {
		t.Fatalf("perr = %v", perr)
	}
	if fk.calls != 0 {
		t.Fatal("runner should not be called")
	}
}

func TestCustomModelRoutesToProvider(t *testing.T) {
	cfg := freshConfig(t)
	if err := cfg.SetModel("my-gemini", config.ModelProfile{Provider: "gemini", StreamMode: config.StreamNative, TimeoutSec: 30, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	fk := &fakeRunner{result: provider.Result{Content: "from gemini"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	res, perr := c.RunChat(context.Background(), provider.Request{Model: "my-gemini", Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
	if perr != nil {
		t.Fatal(perr)
	}
	if res.Content != "from gemini" {
		t.Fatalf("content = %q", res.Content)
	}
	if len(fk.called) != 1 || fk.called[0] != "gemini.exe" {
		t.Fatalf("runner called with %v, want gemini exe", fk.called)
	}
}

func TestStreamDisabledModelRejected(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	_, perr := c.RunChatStream(context.Background(), provider.Request{Model: "codex"}, func(ev provider.StreamEvent) {})
	if perr == nil {
		t.Fatal("expected streaming_not_supported")
	}
	if perr.Code != provider.ErrStreamingNotSupported {
		t.Fatalf("code = %q", perr.Code)
	}
	if fk.calls != 0 {
		t.Fatal("runner must not run for a non-streamable model")
	}
}

func TestStreamHappyPath(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	var got []string
	res, perr := c.RunChatStream(context.Background(), provider.Request{Model: "opencode"}, func(ev provider.StreamEvent) {
		got = append(got, ev.Text)
	})
	if perr != nil {
		t.Fatal(perr)
	}
	if res.Content != "tok1 tok2" {
		t.Fatalf("content = %q", res.Content)
	}
	if len(got) != 2 || got[0] != "tok1 " || got[1] != "tok2" {
		t.Fatalf("deltas = %v", got)
	}
	if len(fk.called) != 1 || fk.called[0] != "opencode.exe" {
		t.Fatalf("runner called with %v", fk.called)
	}
}

func TestSaveAndDeleteModel(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})

	if err := c.SaveModel(ModelInput{ID: "my.model", Provider: "gemini", StreamMode: "native", TimeoutSec: 42, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	m, ok := c.cfg.Model("my.model")
	if !ok || m.StreamMode != config.StreamNative || m.TimeoutSec != 42 || !m.Enabled {
		t.Fatalf("model = %+v", m)
	}
	// Bad provider rejected.
	if err := c.SaveModel(ModelInput{ID: "x", Provider: "nope"}); err == nil {
		t.Fatal("expected unknown provider error")
	}
	// Bad id rejected.
	if err := c.SaveModel(ModelInput{ID: "bad id", Provider: "gemini"}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := c.DeleteModel("my.model"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.cfg.Model("my.model"); ok {
		t.Fatal("model should be gone")
	}
}

func TestEnabledModelsAndSnapshot(t *testing.T) {
	cfg := freshConfig(t)
	if err := cfg.SetModel("hidden", config.ModelProfile{Provider: "codex", Enabled: false, TimeoutSec: 300}); err != nil {
		t.Fatal(err)
	}
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})

	en := c.EnabledModels()
	if len(en) != 4 {
		t.Fatalf("enabled = %v", en)
	}
	for _, id := range en {
		if id == "hidden" {
			t.Fatal("disabled model leaked into EnabledModels")
		}
	}

	snap := c.Snapshot()
	if len(snap.Models) != 5 {
		t.Fatalf("models = %d", len(snap.Models))
	}
	byID := map[string]ModelView{}
	for _, mv := range snap.Models {
		byID[mv.ID] = mv
	}
	if mv := byID["codex"]; mv.Status != "Ready" || !mv.Ready {
		t.Fatalf("codex view = %+v", mv)
	}
	if mv := byID["claude"]; mv.Status != "Backend not installed" || mv.Ready {
		t.Fatalf("claude view = %+v", mv)
	}
	if mv := byID["hidden"]; mv.Status != "Disabled" || mv.Ready {
		t.Fatalf("hidden view = %+v", mv)
	}
}

func TestTestModel(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{result: provider.Result{Content: "OK"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)

	tr := c.TestModel(context.Background(), "codex")
	if !tr.Passed || tr.Response != "OK" {
		t.Fatalf("test = %+v", tr)
	}
	// Test for an unknown model id fails cleanly without running the CLI.
	fk.calls = 0
	tr = c.TestModel(context.Background(), "nonexistent")
	if tr.Passed || tr.Message == "" {
		t.Fatalf("test = %+v", tr)
	}
	if fk.calls != 0 {
		t.Fatal("runner must not run for unknown model")
	}
}
