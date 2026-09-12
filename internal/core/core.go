// Package core coordinates config, CLI discovery, queues, the HTTP server,
// activity logging and optional disk logging. It is the layer the Wails UI
// binds to.
package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"LocalAIProxy/internal/activity"
	"LocalAIProxy/internal/api"
	"LocalAIProxy/internal/config"
	"LocalAIProxy/internal/discovery"
	"LocalAIProxy/internal/logr"
	"LocalAIProxy/internal/proc"
	"LocalAIProxy/internal/provider"
	"LocalAIProxy/internal/queue"
)

// TestResult is captured by the manual Test action.
type TestResult struct {
	Passed    bool   `json:"passed"`
	LatencyMS int64  `json:"latencyMs"`
	Response  string `json:"response,omitempty"`
	Message   string `json:"message,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Time      string `json:"time"`
}

// ProviderView is the read model the Dashboard and Providers tabs render.
type ProviderView struct {
	Alias           string                `json:"alias"`
	Name            string                `json:"name"`
	Enabled         bool                  `json:"enabled"`
	Installed       bool                  `json:"installed"`
	Version         string                `json:"version"`
	Executable      string                `json:"executable"`
	Auth            string                `json:"auth"` // detected | unknown | required
	Status          string                `json:"status"`
	StatusKind      string                `json:"statusKind"` // ok | warn | error | idle
	Ready           bool                  `json:"ready"`
	Concurrency     int                   `json:"concurrency"`
	MaxQueue        int                   `json:"maxQueue"`
	QueueTimeoutSec int                   `json:"queueTimeoutSec"`
	ExecTimeoutSec  int                   `json:"execTimeoutSec"`
	LastTest        *TestResult           `json:"lastTest,omitempty"`
	InstalledBy     string                `json:"installedBy"`
	Capabilities    provider.Capabilities `json:"capabilities"`
	QueueDepth      int                   `json:"queueDepth,omitempty"`
}

// ModelView is the read model the Dashboard renders for one model profile.
type ModelView struct {
	ID            string                `json:"id"`
	Provider      string                `json:"provider"`
	ProviderName  string                `json:"providerName"`
	DisplayName   string                `json:"displayName"`
	UpstreamModel string                `json:"upstreamModel,omitempty"`
	StreamMode    string                `json:"streamMode"`
	TimeoutSec    int                   `json:"timeoutSeconds"`
	Enabled       bool                  `json:"enabled"`
	Status        string                `json:"status"`
	StatusKind    string                `json:"statusKind"` // ok | warn | error | idle
	Ready         bool                  `json:"ready"`
	Capabilities  provider.Capabilities `json:"capabilities,omitempty"`
}

// ModelInput is what the UI sends to create or update a model profile.
type ModelInput struct {
	ID            string   `json:"id"`
	Provider      string   `json:"provider"`
	DisplayName   string   `json:"displayName"`
	UpstreamModel string   `json:"upstreamModel"`
	StreamMode    string   `json:"streamMode"`
	TimeoutSec    int      `json:"timeoutSeconds"`
	Enabled       bool     `json:"enabled"`
	SystemPrompt  string   `json:"systemPrompt"`
	Temperature   *float64 `json:"temperature"`
	MaxTokens     *int     `json:"maxTokens"`
	ContextWindow *int     `json:"contextWindow"`
	ExtraArgs     []string `json:"extraArgs"`
	WorkingDir    string   `json:"workingDir"`
}

// Snapshot is pushed to the UI whenever state changes.
type Snapshot struct {
	ServerRunning  bool             `json:"serverRunning"`
	Host           string           `json:"host"`
	Port           int              `json:"port"`
	URL            string           `json:"url"`
	ConfigURL      string           `json:"configUrl"`
	RequireAPIKey  bool             `json:"requireApiKey"`
	ActiveRequests int32            `json:"activeRequests"`
	Providers      []ProviderView   `json:"providers"`
	Models         []ModelView      `json:"models"`
	Activity       []activity.Entry `json:"activity"`
}

// EventFunc emits UI events from the core.
type EventFunc func(event string, data any)

// Core owns the application runtime.
type Core struct {
	cfg     *config.Config
	homeDir string
	appDir  string

	mu        sync.Mutex
	infos     map[string]discovery.Info
	queues    map[string]*queue.Queue // keyed by PROVIDER alias — all models on one backend share one scheduler
	globalSem chan struct{}           // global safety concurrency (default 1); nil = unbounded (never in practice)
	lastTest  map[string]*TestResult
	runner    provider.Runner
	running   bool
	inFlight  int32

	api    *api.Server
	act    *activity.Log
	logger *logr.Logger
	emit   EventFunc
}

var adapters = map[string]provider.Adapter{
	"claude":   provider.ClaudeAdapter{},
	"codex":    provider.CodexAdapter{},
	"gemini":   provider.GeminiAdapter{},
	"opencode": provider.OpenCodeAdapter{},
}

// New builds a Core from the persisted config, discovers CLIs, and starts
// the HTTP server if auto-start is enabled.
func New(emit EventFunc) (*Core, error) {
	cfg, err := config.FromFile()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	home, _ := os.UserHomeDir()
	appDir, _ := config.AppDataDir()

	runner := &proc.Runner{HomeDir: home}

	c := &Core{
		cfg:      cfg,
		homeDir:  home,
		appDir:   appDir,
		infos:    make(map[string]discovery.Info),
		queues:   make(map[string]*queue.Queue),
		lastTest: make(map[string]*TestResult),
		runner:   runner,
		act:      activity.New(50),
		emit:     emit,
	}
	c.resetGlobalSemLocked(cfg.GlobalConcurrency)

	if logger, err := logr.New(appDir, cfg.RetentionDays, cfg.DebugLogging); err == nil {
		c.logger = logger
	}

	c.RefreshDiscovery()

	c.api = api.New(c, c.act, c.logger)
	if c.cfg.AutoStartServer {
		_ = c.StartServer()
	}
	return c, nil
}

// Rebuild queues from current provider config. Queues are lazy per PROVIDER,
// so this merely drops the cache — the next request recreates the queue it needs.
// Multiple model profiles on the same provider share one scheduler, so per-model
// queues can never bypass provider concurrency.
func (c *Core) rebuildQueues() {
	c.mu.Lock()
	c.queues = make(map[string]*queue.Queue)
	c.resetGlobalSemLocked(c.cfg.GlobalConcurrency)
	c.mu.Unlock()
}

func (c *Core) rebuildQueuesLocked() {
	c.queues = make(map[string]*queue.Queue)
	c.resetGlobalSemLocked(c.cfg.GlobalConcurrency)
}

func (c *Core) resetGlobalSemLocked(n int) {
	if n < 1 {
		n = config.DefaultGlobalConcurrency
	}
	if n > config.MaxGlobalConcurrency {
		n = config.MaxGlobalConcurrency
	}
	c.globalSem = make(chan struct{}, n)
}

// acquireGlobal takes one global safety slot, respecting client cancellation.
// All requests beyond the limit queue here — never dropped.
func (c *Core) acquireGlobal(ctx context.Context) (func(), *provider.Error) {
	c.mu.Lock()
	sem := c.globalSem
	c.mu.Unlock()
	if sem == nil {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, provider.NewError(provider.ErrRequestCancelled, "", "Request cancelled by the client.", 499)
	}
}

func providerFor(alias string) provider.Adapter {
	if a, ok := adapters[alias]; ok {
		return a
	}
	return provider.ClaudeAdapter{}
}

// --- discovery -------------------------------------------------------------

// RefreshDiscovery re-probes installed CLIs and their auth (never any AI request).
func (c *Core) RefreshDiscovery() {
	// Probe all CLIs in parallel; per-alias probes die after 15s max, so a
	// hung --version/--auth-status can never stall the whole app for long.
	results := make(map[string]discovery.Info)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, alias := range config.Aliases() {
		wg.Add(1)
		go func(alias string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			inf := discovery.Probe(ctx, alias, providerFor(alias).Name())
			mu.Lock()
			results[alias] = inf
			mu.Unlock()
		}(alias)
	}
	wg.Wait()

	c.mu.Lock()
	c.infos = results
	c.rebuildQueuesLocked()
	c.mu.Unlock()
	c.emitState()
}

func (c *Core) info(alias string) discovery.Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.infos[alias]
}

// InstalledBy suggests a product name for missing CLIs.
func installedBy(alias string) string {
	switch alias {
	case "claude":
		return "npm install -g @anthropic-ai/claude-code"
	case "codex":
		return "native installer from the OpenAI Codex docs"
	case "gemini":
		return "npm install -g @google/gemini-cli"
	case "opencode":
		return "opencode's installer at https://opencode.ai"
	}
	return ""
}

// --- server lifecycle ------------------------------------------------------

// StartServer binds the HTTP listener. A non-nil error means the port is in
// use (or otherwise unavailable) — we never silently pick another port.
func (c *Core) StartServer() error {
	c.mu.Lock()
	if c.api == nil {
		c.api = api.New(c, c.act, c.logger)
	}
	apiServer := c.api
	c.mu.Unlock()
	if err := apiServer.Start(); err != nil {
		return err
	}
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	c.log("INFO", "server started on %s:%d", c.cfg.Server.Host, c.cfg.Server.Port)
	c.emitState()
	return nil
}

// StopServer shuts the server down and cancels in-flight proxy requests.
func (c *Core) StopServer() {
	c.mu.Lock()
	srv := c.api
	c.mu.Unlock()
	if srv == nil {
		c.emitState()
		return
	}
	srv.Stop()
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	c.log("INFO", "server stopped")
	c.emitState()
}

// RestartServer stops and starts; used by the UI Retry/Refresh path.
func (c *Core) RestartServer() error {
	c.StopServer()
	return c.StartServer()
}

// IsRunning reports whether the HTTP server is currently serving.
func (c *Core) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// --- HTTP backend interface (api.Backend) ----------------------------------

func (c *Core) Port() int            { return c.cfg.Server.Port }
func (c *Core) Host() string         { return c.cfg.Server.Host }
func (c *Core) Aliases() []string    { return config.Aliases() }
func (c *Core) RequiresAPIKey() bool { return c.cfg.RequireAPIKey }
func (c *Core) ValidAPIKey(t string) bool {
	if !c.cfg.RequireAPIKey {
		return true
	}
	if c.cfg.APIKey == "" || t == "" {
		return false
	}
	// Exact case-sensitive constant-time comparison.
	a := []byte(t)
	b := []byte(c.cfg.APIKey)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
func (c *Core) GlobalConcurrency() int { return c.cfg.GlobalConcurrency }

// ModelCheck answers whether a model profile exists and can be requested.
func (c *Core) ModelCheck(id string) api.ModelCheck {
	m, ok := c.cfg.Model(id)
	if !ok {
		return api.ModelCheck{}
	}
	return api.ModelCheck{Exists: true, Enabled: m.Enabled, Provider: m.Provider}
}

// EnabledModels returns ids of enabled model profiles (what /v1/models lists).
func (c *Core) EnabledModels() []string {
	ids := c.cfg.ModelIDs()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if m, ok := c.cfg.Model(id); ok && m.Enabled {
			out = append(out, id)
		}
	}
	return out
}

func (c *Core) ProviderInfo(alias string) api.ProviderInfo {
	ad := providerFor(alias)
	inf := c.info(alias)
	pcfg := c.cfg.Provider(alias)

	auth := string(inf.Auth)
	if !inf.Installed {
		auth = "unknown"
	}
	ready := inf.Installed && pcfg.Enabled && auth != "required"
	return api.ProviderInfo{
		Alias:          alias,
		Name:           ad.Name(),
		Enabled:        pcfg.Enabled,
		Installed:      inf.Installed,
		Version:        inf.Version,
		Authentication: auth,
		Ready:          ready,
	}
}

// RunChat routes one stateless request to exactly the provider behind the
// requested model profile. Never falls back to another provider.
// Concurrency is enforced at two levels: global safety semaphore first,
// then the shared per-PROVIDER queue (all models on one backend share it).
func (c *Core) RunChat(ctx context.Context, req provider.Request) (provider.Result, *provider.Error) {
	atomic.AddInt32(&c.inFlight, 1)
	defer atomic.AddInt32(&c.inFlight, -1)

	ad, inf, _, profile, perr := c.resolveModel(req.Model)
	if perr != nil {
		return provider.Result{}, perr
	}
	req = enrichRequest(req, profile)

	inv, err := ad.Invoke(req)
	if err != nil {
		return provider.Result{}, invokeError(req.Model, err)
	}
	inv.Exec = inf.Executable

	releaseGlobal, gerr := c.acquireGlobal(ctx)
	if gerr != nil {
		return provider.Result{}, gerr
	}
	defer releaseGlobal()

	q := c.queueFor(profile.Provider, profile)
	res, runErr := q.Submit(ctx, inv)
	if runErr != nil {
		return provider.Result{}, normalizeRunErr(runErr, req.Model)
	}
	return res, nil
}

// RunChatStream is RunChat for streaming requests. It requires the model
// profile to allow native streaming; models with stream_mode "disabled" get a
// streaming_not_supported error — never a silent conversion to one-shot JSON.
func (c *Core) RunChatStream(ctx context.Context, req provider.Request, emit func(provider.StreamEvent)) (provider.Result, *provider.Error) {
	atomic.AddInt32(&c.inFlight, 1)
	defer atomic.AddInt32(&c.inFlight, -1)

	ad, inf, _, profile, perr := c.resolveModel(req.Model)
	if perr != nil {
		return provider.Result{}, perr
	}
	if profile.StreamMode != config.StreamNative {
		e := provider.NewError(provider.ErrStreamingNotSupported, req.Model,
			fmt.Sprintf("Model %q does not allow streaming. Set stream=false, or enable streaming in LocalAIProxy.", req.Model), 400)
		return provider.Result{}, e
	}
	if !ad.Capabilities().Streaming {
		e := provider.NewError(provider.ErrStreamingNotSupported, req.Model,
			fmt.Sprintf("Model %q cannot stream on the %s backend. Set stream=false.", req.Model, ad.DisplayName()), 400)
		return provider.Result{}, e
	}
	req = enrichRequest(req, profile)

	inv, err := ad.StreamInvoke(req)
	if err != nil {
		return provider.Result{}, invokeError(req.Model, err)
	}
	if inv.StreamParse == nil {
		// Adapter without a streaming representation on this backend.
		e := provider.NewError(provider.ErrStreamingNotSupported, req.Model,
			fmt.Sprintf("Model %q cannot stream on the %s backend. Set stream=false.", req.Model, ad.DisplayName()), 400)
		return provider.Result{}, e
	}
	inv.Exec = inf.Executable

	releaseGlobal, gerr := c.acquireGlobal(ctx)
	if gerr != nil {
		return provider.Result{}, gerr
	}
	defer releaseGlobal()

	q := c.queueFor(profile.Provider, profile)
	res, runErr := q.SubmitStream(ctx, inv, emit)
	if runErr != nil {
		return provider.Result{}, normalizeRunErr(runErr, req.Model)
	}
	return res, nil
}

// enrichRequest applies ModelProfile policy onto the internal request:
// upstream model selection, system prompt, temperature, caps.
func enrichRequest(req provider.Request, profile config.ModelProfile) provider.Request {
	if req.UpstreamModel == "" {
		req.UpstreamModel = profile.UpstreamModel
	}
	if req.SystemPrompt == "" {
		req.SystemPrompt = profile.SystemPrompt
	}
	if req.Temperature == nil {
		req.Temperature = profile.Temperature
	}
	if req.MaxTokens == nil {
		req.MaxTokens = profile.MaxTokens
	}
	if req.ContextWindow == nil {
		req.ContextWindow = profile.ContextWindow
	}
	if len(req.ExtraArgs) == 0 && len(profile.ExtraArgs) > 0 {
		req.ExtraArgs = profile.ExtraArgs
	}
	if req.WorkingDir == "" {
		req.WorkingDir = profile.WorkingDir
	}
	return req
}

// resolveModel validates a model profile id and returns its backend +
// discovery info + effective queue config. Errors are normalized.
func (c *Core) resolveModel(id string) (provider.Adapter, discovery.Info, config.ProviderConfig, config.ModelProfile, *provider.Error) {
	profile, ok := c.cfg.Model(id)
	if !ok {
		e := provider.NewError(provider.ErrModelNotFound, id, fmt.Sprintf("Unknown model %q.", id), 400)
		return nil, discovery.Info{}, config.ProviderConfig{}, profile, e
	}
	if !profile.Enabled {
		e := provider.NewError(provider.ErrProviderDisabled, id, fmt.Sprintf("Model %q is disabled in LocalAIProxy settings.", id), 400)
		return nil, discovery.Info{}, config.ProviderConfig{}, profile, e
	}
	ad, inf, pcfg, perr := c.resolveProvider(profile.Provider)
	if perr != nil {
		return nil, discovery.Info{}, config.ProviderConfig{}, profile, perr
	}
	return ad, inf, pcfg, profile, nil
}

// resolveProvider validates a provider alias (known, enabled, installed).
func (c *Core) resolveProvider(alias string) (provider.Adapter, discovery.Info, config.ProviderConfig, *provider.Error) {
	ad, known := adapters[alias]
	if !known {
		e := provider.NewError(provider.ErrInvalidRequest, alias, fmt.Sprintf("Unknown provider %q.", alias), 400)
		return nil, discovery.Info{}, config.ProviderConfig{}, e
	}
	pcfg := c.cfg.Provider(alias)
	if !pcfg.Enabled {
		e := provider.NewError(provider.ErrProviderDisabled, alias,
			fmt.Sprintf("%s is disabled in LocalAIProxy settings. Enable it to use its models.", ad.Name()), 400)
		return ad, discovery.Info{}, pcfg, e
	}
	inf := c.info(alias)
	if !inf.Installed || inf.Executable == "" {
		e := provider.NewError(provider.ErrProviderUnavailable, alias,
			fmt.Sprintf("%s is not installed on this machine. Install it, then Refresh.", ad.Name()), 503)
		e.WithDetails(installedBy(alias))
		return ad, inf, pcfg, e
	}
	return ad, inf, pcfg, nil
}

// invokeError maps adapter build failures (e.g. prompt too long) to errors.
func invokeError(model string, err error) *provider.Error {
	if err == provider.ErrPromptTooLong {
		return provider.NewError(provider.ErrInvalidRequest, model, "The message sequence is too long to pass to this CLI safely.", 400)
	}
	return provider.NewError(provider.ErrProviderProcess, model, "The request could not be prepared.", 500)
}

func normalizeRunErr(runErr error, model string) *provider.Error {
	var pe *provider.Error
	if provider.AsError(runErr, &pe) {
		if pe.Provider == "" {
			pe.Provider = model
		}
		if pe.Code == provider.ErrRequestCancelled {
			pe.Status = 499
		}
		return pe
	}
	return provider.NewError(provider.ErrProviderProcess, model, "The provider request failed.", 502)
}

// queueFor returns the shared queue for one PROVIDER alias, creating it
// lazily. All model profiles on the same provider share this scheduler, so
// provider concurrency can never be bypassed by creating more models.
// Exec timeout uses the longest timeout among models on that provider would be
// ideal; we use the requesting profile's timeout for the per-request exec cap
// by wrapping ctx, while the queue itself uses the provider exec default.
func (c *Core) queueFor(providerAlias string, profile config.ModelProfile) *queue.Queue {
	c.mu.Lock()
	defer c.mu.Unlock()
	if q, ok := c.queues[providerAlias]; ok {
		return q
	}
	p := c.cfg.Provider(providerAlias)
	execSec := p.ExecTimeoutSec
	if execSec <= 0 {
		execSec = profile.TimeoutSec
	}
	q := queue.New(providerAlias, queue.Config{
		Concurrency:  p.Concurrency,
		MaxQueue:     p.MaxQueue,
		QueueTimeout: time.Duration(p.QueueTimeoutSec) * time.Second,
		ExecTimeout:  time.Duration(execSec) * time.Second,
	}, c.runner)
	c.queues[providerAlias] = q
	return q
}

// ActiveRequests reports how many provider requests are currently in flight.
func (c *Core) ActiveRequests() int32 {
	return atomic.LoadInt32(&c.inFlight)
}

// --- manual Test -----------------------------------------------------------

// TestProvider runs a tiny real request ("Reply exactly: OK") against one
// provider backend. This is the only path that spends a small amount of quota.
func (c *Core) TestProvider(ctx context.Context, alias string) TestResult {
	ad, inf, pcfg, perr := c.resolveProvider(alias)
	if perr != nil {
		return testResult(alias, 0, perr.Message, perr.Details)
	}
	req := provider.Request{
		Model: alias,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Reply exactly: OK"},
		},
	}
	inv, err := ad.Invoke(req)
	if err != nil {
		pe := invokeError(alias, err)
		return testResult(alias, 0, pe.Message, "")
	}
	inv.Exec = inf.Executable

	start := time.Now()
	q := c.queueFor(alias, config.ModelProfile{Provider: alias, TimeoutSec: pcfg.ExecTimeoutSec})
	atomic.AddInt32(&c.inFlight, 1)
	res, runErr := q.Submit(ctx, inv)
	atomic.AddInt32(&c.inFlight, -1)

	tr := TestResult{
		LatencyMS: time.Since(start).Milliseconds(),
		Time:      time.Now().Format("15:04:05"),
	}
	if runErr != nil {
		pe := normalizeRunErr(runErr, alias)
		tr.Passed = false
		tr.Message = pe.Message
		tr.Detail = pe.Details
	} else {
		tr.Passed = true
		tr.Response = truncate(res.Content, 140)
	}

	c.mu.Lock()
	c.lastTest[alias] = &tr
	c.mu.Unlock()
	c.log("INFO", "provider test %s = %v (%dms)", alias, runErr == nil, tr.LatencyMS)
	c.emitState()
	return tr
}

// TestModel runs the tiny real request through a model profile. Same quota
// cost as TestProvider; used from the Dashboard model table.
func (c *Core) TestModel(ctx context.Context, id string) TestResult {
	ad, inf, _, profile, perr := c.resolveModel(id)
	if perr != nil {
		return testResult(id, 0, perr.Message, perr.Details)
	}
	req := provider.Request{
		Model: id,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Reply exactly: OK"},
		},
	}
	inv, err := ad.Invoke(req)
	if err != nil {
		pe := invokeError(id, err)
		return testResult(id, 0, pe.Message, "")
	}
	inv.Exec = inf.Executable

	start := time.Now()
	q := c.queueFor(id, profile)
	res, runErr := q.Submit(ctx, inv)

	tr := TestResult{
		LatencyMS: time.Since(start).Milliseconds(),
		Time:      time.Now().Format("15:04:05"),
	}
	if runErr != nil {
		pe := normalizeRunErr(runErr, id)
		tr.Passed = false
		tr.Message = pe.Message
		tr.Detail = pe.Details
	} else {
		tr.Passed = true
		tr.Response = truncate(res.Content, 140)
	}

	c.mu.Lock()
	c.lastTest[id] = &tr
	c.mu.Unlock()
	c.log("INFO", "model test %s = %v (%dms)", id, runErr == nil, tr.LatencyMS)
	c.emitState()
	return tr
}

func testResult(alias string, latency int64, msg, detail string) TestResult {
	_ = alias
	return TestResult{
		LatencyMS: latency,
		Time:      time.Now().Format("15:04:05"),
		Message:   msg,
		Detail:    detail,
	}
}

// LastTest returns the most recent test result for a provider.
func (c *Core) LastTest(alias string) *TestResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastTest[alias]
}

func truncate(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

// --- UI helpers ------------------------------------------------------------

// Snapshot renders the current state for the UI.
func (c *Core) Snapshot() Snapshot {
	port := c.cfg.Server.Port
	host := c.cfg.Server.Host
	if host == "" {
		host = config.DefaultHost
	}
	base := fmt.Sprintf("http://%s:%d/v1", host, port)

	providers := make([]ProviderView, 0, len(config.Aliases()))
	for _, alias := range config.Aliases() {
		ad := providerFor(alias)
		inf := c.info(alias)
		pcfg := c.cfg.Provider(alias)
		v := ProviderView{
			Alias:           alias,
			Name:            ad.Name(),
			Enabled:         pcfg.Enabled,
			Installed:       inf.Installed,
			Version:         inf.Version,
			Executable:      inf.Executable,
			Auth:            string(inf.Auth),
			Concurrency:     pcfg.Concurrency,
			MaxQueue:        pcfg.MaxQueue,
			QueueTimeoutSec: pcfg.QueueTimeoutSec,
			ExecTimeoutSec:  pcfg.ExecTimeoutSec,
			InstalledBy:     installedBy(alias),
			Capabilities:    ad.Capabilities(),
		}
		v.Status, v.StatusKind = deriveStatus(v)
		v.Ready = v.Installed && v.Enabled && v.Auth != "required"
		c.mu.Lock()
		v.LastTest = c.lastTest[alias]
		if q, ok := c.queues[alias]; ok && q != nil {
			v.QueueDepth = q.Depth()
		}
		c.mu.Unlock()
		providers = append(providers, v)
	}

	models := make([]ModelView, 0, len(c.cfg.ModelIDs()))
	for _, id := range c.cfg.ModelIDs() {
		models = append(models, c.modelView(id))
	}

	return Snapshot{
		ServerRunning:  c.IsRunning(),
		Host:           host,
		Port:           port,
		URL:            base,
		ConfigURL:      base,
		RequireAPIKey:  c.cfg.RequireAPIKey,
		ActiveRequests: atomic.LoadInt32(&c.inFlight),
		Providers:      providers,
		Models:         models,
		Activity:       c.act.List(),
	}
}

// modelView builds the read model for one profile id.
func (c *Core) modelView(id string) ModelView {
	m, _ := c.cfg.Model(id)
	ad := providerFor(m.Provider)
	inf := c.info(m.Provider)
	v := ModelView{
		ID:            id,
		Provider:      m.Provider,
		ProviderName:  ad.Name(),
		DisplayName:   m.DisplayName,
		UpstreamModel: m.UpstreamModel,
		StreamMode:    string(m.StreamMode),
		TimeoutSec:    m.TimeoutSec,
		Enabled:       m.Enabled,
		Capabilities:  ad.Capabilities(),
	}
	v.Status, v.StatusKind = deriveModelStatus(v, inf.Installed, inf.Auth)
	v.Ready = v.Enabled && v.StatusKind == "ok"
	return v
}

// deriveModelStatus maps a profile's runtime state to a label + kind.
func deriveModelStatus(v ModelView, installed bool, auth discovery.AuthState) (string, string) {
	switch {
	case !v.Enabled:
		return "Disabled", "idle"
	case !installed:
		return "Backend not installed", "error"
	case string(auth) == "required":
		return "Auth required", "error"
	case string(auth) == "unknown":
		return "Auth unknown", "warn"
	default:
		return "Ready", "ok"
	}
}

// Models returns read models for every profile id (stable order).
func (c *Core) Models() []ModelView {
	models := make([]ModelView, 0, len(c.cfg.ModelIDs()))
	for _, id := range c.cfg.ModelIDs() {
		models = append(models, c.modelView(id))
	}
	return models
}

// deriveStatus maps raw state to a friendly label + kind (never color-only).
func deriveStatus(v ProviderView) (string, string) {
	switch {
	case !v.Installed:
		return "Not installed", "error"
	case !v.Enabled:
		return "Disabled", "idle"
	case v.Auth == "required":
		return "Auth required", "error"
	case v.Auth == "unknown":
		return "Auth unknown", "warn"
	default:
		return "Ready", "ok"
	}
}

// --- settings mutation -----------------------------------------------------

// SetPort applies a new port safely: validate -> test-bind new port ->
// switch -> persist. If binding the new port fails, the working server stays
// alive and UI state never diverges from the actual listener.
func (c *Core) SetPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if err := c.applyPort(port); err != nil {
		return err
	}
	c.emitState()
	return nil
}

// PortInUse reports whether something already listens on the address. The
// proxy itself counts (it binds the same address), which is what the UI wants.
func (c *Core) PortInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.cfg.Server.Host, port), 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// SaveModel creates or updates a model profile. Provider queues are shared,
// so no per-model queue eviction is needed; global + provider schedulers pick
// up new timeouts on the next request.
func (c *Core) SaveModel(in ModelInput) error {
	if !containsAlias(in.Provider) {
		return fmt.Errorf("unknown provider %q", in.Provider)
	}
	// Honest capability policy: native streaming requires provider support.
	ad := providerFor(in.Provider)
	if config.ParseStreamMode(in.StreamMode) == config.StreamNative && !ad.Capabilities().Streaming {
		return fmt.Errorf("provider %q does not support native streaming; use Disabled", in.Provider)
	}
	profile := config.ModelProfile{
		Provider:      in.Provider,
		DisplayName:   in.DisplayName,
		UpstreamModel: in.UpstreamModel,
		StreamMode:    config.ParseStreamMode(in.StreamMode),
		TimeoutSec:    in.TimeoutSec,
		Enabled:       in.Enabled,
		SystemPrompt:  in.SystemPrompt,
		Temperature:   in.Temperature,
		MaxTokens:     in.MaxTokens,
		ContextWindow: in.ContextWindow,
		ExtraArgs:     in.ExtraArgs,
		WorkingDir:    in.WorkingDir,
	}
	if err := c.cfg.SetModel(in.ID, profile); err != nil {
		return err
	}
	c.log("INFO", "model %q saved (provider %s, stream=%s)", in.ID, in.Provider, profile.StreamMode)
	c.emitState()
	return nil
}

// DeleteModel removes a model profile (and its queue cache) and persists.
func (c *Core) DeleteModel(id string) error {
	if err := c.cfg.DeleteModel(id); err != nil {
		return err
	}
	c.log("INFO", "model %q deleted", id)
	c.emitState()
	return nil
}

// DuplicateModel copies src profile to dst id.
func (c *Core) DuplicateModel(srcID, dstID string) error {
	src, ok := c.cfg.Model(srcID)
	if !ok {
		return fmt.Errorf("unknown model %q", srcID)
	}
	if err := c.cfg.SetModel(dstID, src); err != nil {
		return err
	}
	c.log("INFO", "model %q duplicated to %q", srcID, dstID)
	c.emitState()
	return nil
}

func containsAlias(a string) bool {
	for _, x := range config.Aliases() {
		if x == a {
			return true
		}
	}
	return false
}

func (c *Core) SetAutoStart(b bool) error {
	c.cfg.AutoStartServer = b
	if err := c.cfg.Save(); err != nil {
		return err
	}
	return nil
}

// GenerateAPIKey creates a fresh random API key and persists it.
func (c *Core) GenerateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := "lap-" + hex.EncodeToString(b)
	c.cfg.APIKey = key
	if err := c.cfg.Save(); err != nil {
		return "", err
	}
	c.log("INFO", "generated new API key")
	return key, nil
}

func (c *Core) SetAPIKeyEnabled(b bool) error {
	c.cfg.RequireAPIKey = b
	if b && c.cfg.APIKey == "" {
		if _, err := c.GenerateAPIKey(); err != nil {
			return err
		}
	}
	if err := c.cfg.Save(); err != nil {
		return err
	}
	c.emitState()
	return nil
}

// CurrentAPIKey returns the persisted local API key (empty when none set).
func (c *Core) CurrentAPIKey() string { return c.cfg.APIKey }

func (c *Core) SetLogging(save bool, retention int, debug bool) error {
	c.cfg.SaveLogsToDisk = save
	c.cfg.RetentionDays = retention
	c.cfg.DebugLogging = debug
	if save {
		if c.logger == nil {
			if l, err := logr.New(c.appDir, retention, debug); err == nil {
				c.logger = l
			}
		} else {
			c.logger.SetDebug(debug)
		}
		c.log("INFO", "disk logging enabled (retention %dd)", retention)
	} else if c.logger != nil {
		_ = c.logger.Close()
		c.logger = nil
	}
	if err := c.cfg.Save(); err != nil {
		return err
	}
	return nil
}

// SaveProviderSettings updates one provider and rebuilds queues.
func (c *Core) SaveProviderSettings(alias string, p providerSettings) error {
	cur := c.cfg.Provider(alias)
	if p.Enabled != nil {
		cur.Enabled = *p.Enabled
	}
	if p.Concurrency != nil && *p.Concurrency >= 1 {
		cur.Concurrency = *p.Concurrency
	}
	if p.MaxQueue != nil && *p.MaxQueue >= 0 {
		cur.MaxQueue = *p.MaxQueue
	}
	if p.QueueTimeoutSec != nil && *p.QueueTimeoutSec >= 0 {
		cur.QueueTimeoutSec = *p.QueueTimeoutSec
	}
	if p.ExecTimeoutSec != nil && *p.ExecTimeoutSec >= 0 {
		cur.ExecTimeoutSec = *p.ExecTimeoutSec
	}
	if err := c.rebuildAndSave(alias, cur); err != nil {
		return err
	}
	c.emitState()
	return nil
}

type providerSettings struct {
	Enabled         *bool `json:"enabled"`
	Concurrency     *int  `json:"concurrency"`
	MaxQueue        *int  `json:"maxQueue"`
	QueueTimeoutSec *int  `json:"queueTimeoutSec"`
	ExecTimeoutSec  *int  `json:"execTimeoutSec"`
}

// ProviderSettingsFromMap converts a JS settings object into providerSettings.
func ProviderSettingsFromMap(m map[string]any) providerSettings {
	ps := providerSettings{}
	if b, ok := m["enabled"].(bool); ok {
		ps.Enabled = &b
	}
	for _, spec := range []struct {
		key string
		dst **int
	}{
		{"concurrency", &ps.Concurrency},
		{"maxQueue", &ps.MaxQueue},
		{"queueTimeoutSec", &ps.QueueTimeoutSec},
		{"execTimeoutSec", &ps.ExecTimeoutSec},
	} {
		if f, ok := m[spec.key].(float64); ok && int(f) >= 0 {
			v := int(f)
			*spec.dst = &v
		}
	}
	return ps
}

func (c *Core) rebuildAndSave(alias string, p config.ProviderConfig) error {
	c.cfg.SetProvider(alias, p)
	c.rebuildQueues()
	return nil
}

// RestoreDefaults wipes persisted settings back to defaults (key cleared).
func (c *Core) RestoreDefaults() error {
	def := config.Default()
	def.Save()
	c.cfg = def
	c.rebuildQueues()
	c.emitState()
	return nil
}

// DismissFirstRun marks the first-run guide as seen.
func (c *Core) DismissFirstRun() error {
	c.cfg.FirstRunDismissed = true
	return c.cfg.Save()
}

// ResetFirstRun re-shows the first-run guide next launch.
func (c *Core) ResetFirstRun() error {
	c.cfg.FirstRunDismissed = false
	return c.cfg.Save()
}

// IsFirstRun reports whether to show the guide.
func (c *Core) IsFirstRun() bool { return !c.cfg.FirstRunDismissed }

// --- misc ------------------------------------------------------------------

func (c *Core) emitState() {
	if c.emit != nil {
		c.emit("state", c.Snapshot())
	}
}

func (c *Core) log(level, format string, args ...any) {
	if c.logger != nil {
		switch level {
		case "ERROR":
			c.logger.Error(format, args...)
		case "DEBUG":
			c.logger.Debug(format, args...)
		default:
			c.logger.Info(format, args...)
		}
	}
}

// Activities returns the recent activity list (copy).
func (c *Core) Activities() []activity.Entry { return c.act.List() }

// Config returns the current persisted config (read snapshot, not the live one).
func (c *Core) Config() map[string]any {
	// Guard against UI rendering internals: expose a small safe view.
	view := map[string]any{
		"port":              c.cfg.Server.Port,
		"host":              c.cfg.Server.Host,
		"allowNonLoopback":  c.cfg.Server.AllowNonLoopback,
		"autoStartServer":   c.cfg.AutoStartServer,
		"requireApiKey":     c.cfg.RequireAPIKey,
		"firstRunDismissed": c.cfg.FirstRunDismissed,
		"saveLogsToDisk":    c.cfg.SaveLogsToDisk,
		"retentionDays":     c.cfg.RetentionDays,
		"debugLogging":      c.cfg.DebugLogging,
		"globalConcurrency": c.cfg.GlobalConcurrency,
	}
	pv := map[string]any{}
	for _, alias := range config.Aliases() {
		p := c.cfg.Provider(alias)
		pv[alias] = map[string]any{
			"enabled":         p.Enabled,
			"concurrency":     p.Concurrency,
			"maxQueue":        p.MaxQueue,
			"queueTimeoutSec": p.QueueTimeoutSec,
			"execTimeoutSec":  p.ExecTimeoutSec,
		}
	}
	view["providers"] = pv

	mv := map[string]any{}
	for _, id := range c.cfg.ModelIDs() {
		m, _ := c.cfg.Model(id)
		mv[id] = map[string]any{
			"provider":      m.Provider,
			"displayName":   m.DisplayName,
			"upstreamModel": m.UpstreamModel,
			"streamMode":    string(m.StreamMode),
			"timeoutSec":    m.TimeoutSec,
			"enabled":       m.Enabled,
			"systemPrompt":  m.SystemPrompt,
			"extraArgs":     m.ExtraArgs,
			"workingDir":    m.WorkingDir,
		}
		if m.Temperature != nil {
			mv[id].(map[string]any)["temperature"] = *m.Temperature
		}
		if m.MaxTokens != nil {
			mv[id].(map[string]any)["maxTokens"] = *m.MaxTokens
		}
		if m.ContextWindow != nil {
			mv[id].(map[string]any)["contextWindow"] = *m.ContextWindow
		}
	}
	view["models"] = mv
	return view
}

// SetGlobalConcurrency updates the global safety limit (default 1).
func (c *Core) SetGlobalConcurrency(n int) error {
	if n < 1 || n > config.MaxGlobalConcurrency {
		return fmt.Errorf("global concurrency must be 1-%d", config.MaxGlobalConcurrency)
	}
	c.cfg.GlobalConcurrency = n
	if err := c.cfg.Save(); err != nil {
		return err
	}
	c.rebuildQueues()
	c.emitState()
	return nil
}

// Capabilities returns honest provider capabilities for the UI.
func (c *Core) Capabilities(alias string) provider.Capabilities {
	return providerFor(alias).Capabilities()
}

// Configure applies the Settings form in one shot.
// Port changes go through the same safe path as SetPort: pre-bind test,
// switch, restart if running, rollback on failure — UI never diverges from
// the actual listener.
func (c *Core) Configure(settings map[string]any) error {
	newPort := c.cfg.Server.Port
	if v, ok := settings["port"].(float64); ok {
		if v < 1 || v > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		newPort = int(v)
	}
	if v, ok := settings["globalConcurrency"].(float64); ok {
		if int(v) < 1 || int(v) > config.MaxGlobalConcurrency {
			return fmt.Errorf("global concurrency must be 1-%d", config.MaxGlobalConcurrency)
		}
		c.cfg.GlobalConcurrency = int(v)
	}
	if v, ok := settings["allowNonLoopback"].(bool); ok {
		c.cfg.Server.AllowNonLoopback = v
	}
	if err := c.cfg.ValidateHost(); err != nil {
		c.cfg.Server.Host = config.DefaultHost
		c.cfg.Server.AllowNonLoopback = false
		return err
	}
	if v, ok := settings["autoStartServer"].(bool); ok {
		c.cfg.AutoStartServer = v
	}
	if v, ok := settings["saveLogsToDisk"].(bool); ok {
		c.cfg.SaveLogsToDisk = v
	}
	if v, ok := settings["retentionDays"].(float64); ok && v > 0 {
		c.cfg.RetentionDays = int(v)
	}
	if v, ok := settings["debugLogging"].(bool); ok {
		c.cfg.DebugLogging = v
	}
	if v, ok := settings["providers"].(map[string]any); ok {
		for _, alias := range config.Aliases() {
			if pm, ok := v[alias].(map[string]any); ok {
				p := c.cfg.Provider(alias)
				if b, ok := pm["enabled"].(bool); ok {
					p.Enabled = b
				}
				if f, ok := pm["concurrency"].(float64); ok && int(f) >= 1 {
					p.Concurrency = int(f)
				}
				if f, ok := pm["maxQueue"].(float64); ok && int(f) >= 0 {
					p.MaxQueue = int(f)
				}
				if f, ok := pm["queueTimeoutSec"].(float64); ok && int(f) >= 0 {
					p.QueueTimeoutSec = int(f)
				}
				if f, ok := pm["execTimeoutSec"].(float64); ok && int(f) >= 0 {
					p.ExecTimeoutSec = int(f)
				}
				c.cfg.SetProvider(alias, p)
			}
		}
	}
	if newPort != c.cfg.Server.Port {
		if err := c.applyPort(newPort); err != nil {
			return err
		}
		// applyPort already saved + restarted + emitted; finish queue rebuild below.
	} else if err := c.cfg.Save(); err != nil {
		return err
	}
	c.rebuildQueues()
	c.emitState()
	return nil
}

// applyPort is the shared safe port-switch used by SetPort and Configure.
func (c *Core) applyPort(port int) error {
	old := c.cfg.Server.Port
	if port == old {
		return nil
	}
	host := c.cfg.Server.Host
	if host == "" {
		host = config.DefaultHost
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return fmt.Errorf("port %d is already in use. The proxy is still running on port %d", port, old)
	}
	_ = ln.Close()
	c.cfg.Server.Port = port
	if err := c.cfg.Save(); err != nil {
		c.cfg.Server.Port = old
		return err
	}
	if c.IsRunning() {
		if err := c.RestartServer(); err != nil {
			c.cfg.Server.Port = old
			if e2 := c.cfg.Save(); e2 != nil {
				return fmt.Errorf("new port unavailable (%v); rollback save failed: %w", err, e2)
			}
			if e2 := c.StartServer(); e2 != nil {
				return fmt.Errorf("new port unavailable (%v) and old port could not be rebound (%v)", err, e2)
			}
			return fmt.Errorf("new port unavailable: %v (reverted to port %d)", err, old)
		}
	}
	c.log("INFO", "port changed to %d", port)
	return nil
}

// LogPath returns where logs live, for UI display.
func (c *Core) LogPath() string {
	return filepath.Join(c.appDir, "logs")
}

// Shutdown stops the server and logging cleanly. Safe to call once.
func (c *Core) Shutdown() {
	c.StopServer()
	if c.logger != nil {
		_ = c.logger.Close()
	}
}

// ServerURL returns the dashboard's base URL.
func (c *Core) ServerURL() string {
	host := c.cfg.Server.Host
	if host == "" {
		host = config.DefaultHost
	}
	return fmt.Sprintf("http://%s:%d/v1", host, c.cfg.Server.Port)
}
