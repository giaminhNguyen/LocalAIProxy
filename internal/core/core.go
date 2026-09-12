// Package core coordinates config, CLI discovery, queues, the HTTP server,
// activity logging and optional disk logging. It is the layer the Wails UI
// binds to.
package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	Alias           string      `json:"alias"`
	Name            string      `json:"name"`
	Enabled         bool        `json:"enabled"`
	Installed       bool        `json:"installed"`
	Version         string      `json:"version"`
	Executable      string      `json:"executable"`
	Auth            string      `json:"auth"` // detected | unknown | required
	Status          string      `json:"status"`
	StatusKind      string      `json:"statusKind"` // ok | warn | error | idle
	Ready           bool        `json:"ready"`
	Concurrency     int         `json:"concurrency"`
	MaxQueue        int         `json:"maxQueue"`
	QueueTimeoutSec int         `json:"queueTimeoutSec"`
	ExecTimeoutSec  int         `json:"execTimeoutSec"`
	LastTest        *TestResult `json:"lastTest,omitempty"`
	InstalledBy     string      `json:"installedBy"`
}

// Snapshot is pushed to the UI whenever state changes.
type Snapshot struct {
	ServerRunning bool             `json:"serverRunning"`
	Port          int              `json:"port"`
	URL           string           `json:"url"`
	ConfigURL     string           `json:"configUrl"`
	Providers     []ProviderView   `json:"providers"`
	Activity      []activity.Entry `json:"activity"`
}

// EventFunc emits UI events from the core.
type EventFunc func(event string, data any)

// Core owns the application runtime.
type Core struct {
	cfg     *config.Config
	homeDir string
	appDir  string

	mu       sync.Mutex
	infos    map[string]discovery.Info
	queues   map[string]*queue.Queue
	lastTest map[string]*TestResult
	runner   provider.Runner
	running  bool
	inFlight int32

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

// Rebuild queues from current provider config.
func (c *Core) rebuildQueues() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, alias := range config.Aliases() {
		p := c.cfg.Provider(alias)
		c.queues[alias] = queue.New(displayName(alias), queue.Config{
			Concurrency:  p.Concurrency,
			MaxQueue:     p.MaxQueue,
			QueueTimeout: time.Duration(p.QueueTimeoutSec) * time.Second,
			ExecTimeout:  time.Duration(p.ExecTimeoutSec) * time.Second,
		}, c.runner)
	}
}

func displayName(alias string) string {
	return providerFor(alias).Name()
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

func (c *Core) rebuildQueuesLocked() {
	for _, alias := range config.Aliases() {
		p := c.cfg.Provider(alias)
		c.queues[alias] = queue.New(displayName(alias), queue.Config{
			Concurrency:  p.Concurrency,
			MaxQueue:     p.MaxQueue,
			QueueTimeout: time.Duration(p.QueueTimeoutSec) * time.Second,
			ExecTimeout:  time.Duration(p.ExecTimeoutSec) * time.Second,
		}, c.runner)
	}
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
	c.log("INFO", "server started on 127.0.0.1:%d", c.cfg.Port)
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

func (c *Core) Port() int            { return c.cfg.Port }
func (c *Core) Aliases() []string    { return config.Aliases() }
func (c *Core) RequiresAPIKey() bool { return c.cfg.RequireAPIKey }
func (c *Core) ValidAPIKey(t string) bool {
	return !c.cfg.RequireAPIKey || (c.cfg.APIKey != "" && strings.EqualFold(t, c.cfg.APIKey))
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

// RunChat routes one stateless request to exactly the requested provider CLI.
// Never falls back to another provider.
func (c *Core) RunChat(ctx context.Context, req provider.Request) (provider.Result, *provider.Error) {
	atomic.AddInt32(&c.inFlight, 1)
	defer atomic.AddInt32(&c.inFlight, -1)

	alias := req.Model
	ad := providerFor(alias)
	pcfg := c.cfg.Provider(alias)

	if !pcfg.Enabled {
		e := provider.NewError(provider.ErrProviderDisabled, alias,
			fmt.Sprintf("%s is disabled in LocalAIProxy settings. Enable it to use this model.", ad.Name()), 400)
		c.log("WARN", "request for disabled provider %s", alias)
		return provider.Result{}, e
	}

	inf := c.info(alias)
	if !inf.Installed || inf.Executable == "" {
		e := provider.NewError(provider.ErrProviderUnavailable, alias,
			fmt.Sprintf("%s is not installed on this machine. Install it, then Refresh.", ad.Name()), 503)
		e.WithDetails(installedBy(alias))
		return provider.Result{}, e
	}

	inv, err := ad.Invoke(req)
	if err != nil {
		code := provider.ErrProviderProcess
		msg := "The request could not be prepared."
		status := 500
		if err == provider.ErrPromptTooLong {
			code = provider.ErrInvalidRequest
			msg = "The message sequence is too long to pass to this CLI safely."
			status = 400
		}
		e := provider.NewError(code, alias, msg, status)
		return provider.Result{}, e
	}
	inv.Exec = inf.Executable

	q := c.queueFor(alias)
	res, runErr := q.Submit(ctx, inv)
	if runErr != nil {
		var pe *provider.Error
		if provider.AsError(runErr, &pe) {
			if pe.Provider == "" {
				pe.Provider = alias
			}
			if pe.Code == provider.ErrRequestCancelled {
				pe.Status = 499
			}
			return provider.Result{}, pe
		}
		n := provider.NewError(provider.ErrProviderProcess, alias, "The provider request failed.", 502)
		return provider.Result{}, n
	}
	return res, nil
}

func (c *Core) queueFor(alias string) *queue.Queue {
	c.mu.Lock()
	defer c.mu.Unlock()
	if q, ok := c.queues[alias]; ok {
		return q
	}
	p := c.cfg.Provider(alias)
	q := queue.New(displayName(alias), queue.Config{
		Concurrency:  p.Concurrency,
		MaxQueue:     p.MaxQueue,
		QueueTimeout: time.Duration(p.QueueTimeoutSec) * time.Second,
		ExecTimeout:  time.Duration(p.ExecTimeoutSec) * time.Second,
	}, c.runner)
	c.queues[alias] = q
	return q
}

// ActiveRequests reports how many provider requests are currently in flight.
func (c *Core) ActiveRequests() int32 {
	return atomic.LoadInt32(&c.inFlight)
}

// --- manual Test -----------------------------------------------------------

// TestProvider runs a tiny real request ("Reply exactly: OK") against one
// provider. This is the only path that spends a small amount of quota.
func (c *Core) TestProvider(ctx context.Context, alias string) TestResult {
	start := time.Now()
	req := provider.Request{
		Model: alias,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Reply exactly: OK"},
		},
	}
	res, perr := c.RunChat(ctx, req)
	tr := TestResult{
		LatencyMS: time.Since(start).Milliseconds(),
		Time:      time.Now().Format("15:04:05"),
	}
	if perr != nil {
		tr.Passed = false
		tr.Message = perr.Message
		tr.Detail = perr.Details
	} else {
		tr.Passed = true
		tr.Response = truncate(res.Content, 140)
	}

	c.mu.Lock()
	c.lastTest[alias] = &tr
	c.mu.Unlock()
	c.log("INFO", "provider test %s = %v (%dms)", alias, perr == nil, tr.LatencyMS)
	c.emitState()
	return tr
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
	port := c.cfg.Port
	base := fmt.Sprintf("http://127.0.0.1:%d/v1", port)

	providers := make([]ProviderView, 0, 4)
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
		}
		v.Status, v.StatusKind = deriveStatus(v)
		v.Ready = v.Installed && v.Enabled && v.Auth != "required"
		c.mu.Lock()
		v.LastTest = c.lastTest[alias]
		c.mu.Unlock()
		providers = append(providers, v)
	}

	return Snapshot{
		ServerRunning: c.IsRunning(),
		Port:          port,
		URL:           base,
		ConfigURL:     fmt.Sprintf("http://127.0.0.1:%d/v1", port),
		Providers:     providers,
		Activity:      c.act.List(),
	}
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

func (c *Core) SetPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	c.cfg.Port = port
	if err := c.cfg.Save(); err != nil {
		return err
	}
	c.emitState()
	return nil
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
		"port":              c.cfg.Port,
		"autoStartServer":   c.cfg.AutoStartServer,
		"requireApiKey":     c.cfg.RequireAPIKey,
		"firstRunDismissed": c.cfg.FirstRunDismissed,
		"saveLogsToDisk":    c.cfg.SaveLogsToDisk,
		"retentionDays":     c.cfg.RetentionDays,
		"debugLogging":      c.cfg.DebugLogging,
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
	return view
}

// Configure applies the Settings form in one shot.
func (c *Core) Configure(settings map[string]any) error {
	if v, ok := settings["port"].(float64); ok {
		if v < 1 || v > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		c.cfg.Port = int(v)
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
	if err := c.cfg.Save(); err != nil {
		return err
	}
	c.rebuildQueues()
	c.emitState()
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
	return fmt.Sprintf("http://127.0.0.1:%d/v1", c.cfg.Port)
}
