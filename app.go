// App is the Wails-bound service object the frontend calls.
package main

import (
	"context"
	"sync"

	"LocalAIProxy/internal/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App wraps the core runtime for the UI.
type App struct {
	ctx context.Context

	once    sync.Once // boot is idempotent; all bindings funnel through it
	core    *core.Core
	initErr error
}

// NewApp creates the Wails app shell.
func NewApp() *App {
	return &App{}
}

// startup wires the app to the runtime context. Core construction is deferred
// to the first binding call (boot) so the UI can never observe a half-built core.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// boot initializes the core exactly once. Every binding call goes through it,
// so concurrent UI calls during a slow first boot all wait for the same result.
func (a *App) boot() {
	a.once.Do(func() {
		a.core, a.initErr = core.New(func(event string, data any) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, event, data)
			}
		})
	})
}

// shutdown releases core resources (server stopped, logger closed).
func (a *App) shutdown(ctx context.Context) {
	a.boot()
	if a.core != nil {
		a.core.Shutdown()
	}
}

// beforeClose intercepts window close. If requests are running, it asks the
// UI to confirm instead of silently cutting the CLI processes.
// beforeClose is intentionally permissive. Every CLI child runs inside the
// app's Job Object (KILL_ON_JOB_CLOSE), so when the window closes the OS
// terminates the whole process tree — verified independently of any request
// counter. The old guard that blocked close while ActiveRequests()>0 could
// strand the UI when the counter (inFlight) was left >0 by a prior in-flight
// request, so it was dropped. Drop is safe because there is nothing left to
// "wait for": children never outlive the app.
func (a *App) beforeClose(ctx context.Context) bool {
	return false // always allow close; Job Object reaps the tree
}

// ---- UI-facing methods -----------------------------------------------------

// GetSnapshot returns the full dashboard state.
func (a *App) GetSnapshot() core.Snapshot {
	a.boot()
	if a.core == nil {
		return core.Snapshot{}
	}
	return a.core.Snapshot()
}

// StartServer starts the HTTP server.
func (a *App) StartServer() error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.StartServer()
}
func (a *App) StopServer() {
	a.boot()
	if a.core != nil {
		a.core.StopServer()
	}
}
func (a *App) RestartServer() error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.RestartServer()
}
func (a *App) IsRunning() bool {
	a.boot()
	return a.core != nil && a.core.IsRunning()
}

// TestProvider runs a tiny real request. Spends a very small amount of quota.
func (a *App) TestProvider(alias string) core.TestResult {
	a.boot()
	if a.core == nil {
		return core.TestResult{Message: "app failed to start"}
	}
	return a.core.TestProvider(context.Background(), alias)
}

// TestProviderCancelable runs a test honoring a cancellation signal.
func (a *App) TestProviderCancelable(ctx context.Context, alias string) core.TestResult {
	a.boot()
	if a.core == nil {
		return core.TestResult{Message: "app failed to start"}
	}
	return a.core.TestProvider(ctx, alias)
}

// TestModel runs the tiny real request through one model profile.
func (a *App) TestModel(id string) core.TestResult {
	a.boot()
	if a.core == nil {
		return core.TestResult{Message: "app failed to start"}
	}
	return a.core.TestModel(context.Background(), id)
}

// SaveModel creates or updates a model profile.
func (a *App) SaveModel(input core.ModelInput) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SaveModel(input)
}

// DeleteModel removes a model profile.
func (a *App) DeleteModel(id string) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.DeleteModel(id)
}

// PortInUse reports whether a port is already taken on the proxy host.
func (a *App) PortInUse(port int) bool {
	a.boot()
	if a.core == nil {
		return false
	}
	return a.core.PortInUse(port)
}

// Refresh re-probes installed CLIs and auth state (no AI request).
func (a *App) Refresh() {
	a.boot()
	if a.core != nil {
		a.core.RefreshDiscovery()
	}
}

// First-run guide.
func (a *App) IsFirstRun() bool {
	a.boot()
	return a.core != nil && a.core.IsFirstRun()
}
func (a *App) DismissFirstRun() error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.DismissFirstRun()
}
func (a *App) ResetFirstRun() error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.ResetFirstRun()
}

// Settings.
func (a *App) SetPort(port int) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SetPort(port)
}
func (a *App) SetAutoStart(b bool) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SetAutoStart(b)
}
func (a *App) SetAPIKeyEnabled(b bool) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SetAPIKeyEnabled(b)
}
func (a *App) GenerateAPIKey() (string, error) {
	a.boot()
	if a.initErr != nil {
		return "", a.initErr
	}
	return a.core.GenerateAPIKey()
}

// GetAPIKey returns the current local API key so the UI can copy it. The key
// is local-only and is never logged or sent anywhere.
func (a *App) GetAPIKey() string {
	a.boot()
	if a.core == nil {
		return ""
	}
	return a.core.CurrentAPIKey()
}
func (a *App) SaveLogging(save bool, retention int, debug bool) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SetLogging(save, retention, debug)
}

// SaveProvider updates one provider's enabled/concurrency/queue settings.
func (a *App) SaveProvider(alias string, settings map[string]any) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SaveProviderSettings(alias, core.ProviderSettingsFromMap(settings))
}

// SetGlobalConcurrency updates the global safety limit (default 1).
func (a *App) SetGlobalConcurrency(n int) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.SetGlobalConcurrency(n)
}

// DuplicateModel copies a model profile to a new id.
func (a *App) DuplicateModel(srcID, dstID string) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.DuplicateModel(srcID, dstID)
}

// GetCapabilities returns honest provider capabilities for the UI.
func (a *App) GetCapabilities(alias string) map[string]bool {
	a.boot()
	if a.core == nil {
		return map[string]bool{}
	}
	caps := a.core.Capabilities(alias)
	return map[string]bool{
		"streaming":        caps.Streaming,
		"tools":            caps.Tools,
		"structuredOutput": caps.StructuredOutput,
		"usage":            caps.Usage,
		"vision":           caps.Vision,
		"modelSelection":   caps.ModelSelection,
		"sessions":         caps.Sessions,
	}
}

// Configure applies the full settings form.
func (a *App) Configure(settings map[string]any) error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.Configure(settings)
}

// GetConfig returns a safe view of persisted settings for the Settings tab.
func (a *App) GetConfig() map[string]any {
	a.boot()
	if a.core == nil {
		return map[string]any{}
	}
	return a.core.Config()
}

// RestoreDefaults resets all persisted settings.
func (a *App) RestoreDefaults() error {
	a.boot()
	if a.initErr != nil {
		return a.initErr
	}
	return a.core.RestoreDefaults()
}

// ConfirmClose stops the server and quits for real.
func (a *App) ConfirmClose() {
	a.boot()
	if a.core != nil {
		a.core.Shutdown()
	}
	runtime.Quit(a.ctx)
}
