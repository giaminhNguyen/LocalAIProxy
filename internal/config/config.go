// Package config persists LocalAIProxy application settings.
//
// Settings live in %APPDATA%\LocalAIProxy\config.json.
// Provider OAuth credentials are NEVER stored here - the CLIs own their own auth.
//
// Schema v2 separates three concepts:
//
//	server    -> host + port (what the proxy listens on)
//	providers -> runtime tuning for each CLI backend (concurrency, queueing)
//	models    -> model profiles: what OpenAI-compatible clients see; each
//	            profile routes to one provider backend and owns its own stream
//	            mode / timeout / enabled state
//
// v1 files (top-level "port", providers used directly as models) are migrated
// on load: the port moves under server, and a default model profile is seeded
// for every provider so existing clients keep working.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	SchemaVersion = 3

	DefaultHost          = "127.0.0.1"
	DefaultPort          = 8317
	DefaultModelTimeout  = 300
	DefaultConcurrency   = 1
	DefaultMaxQueue      = 10
	DefaultQueueTimeoutS = 120
	DefaultExecTimeoutS  = 0
	DefaultRetentionDays = 7
	MaxTimeoutSec        = 86400

	DefaultGlobalConcurrency = 1
	MaxGlobalConcurrency     = 64
)

// StreamMode is the streaming policy of one model profile.
type StreamMode string

const (
	// StreamDisabled rejects "stream": true with streaming_not_supported.
	StreamDisabled StreamMode = "disabled"
	// StreamNative returns OpenAI-compatible SSE chunks for "stream": true.
	StreamNative StreamMode = "native"
)

// ParseStreamMode normalizes a user-supplied stream mode (invalid -> disabled).
func ParseStreamMode(s string) StreamMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "native":
		return StreamNative
	default:
		return StreamDisabled
	}
}

// ServerConfig owns the HTTP listener address. Local-only by default.
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// AllowNonLoopback is an explicit opt-in to bind a non-loopback address.
	// Default false: non-loopback hosts are rejected and reset to 127.0.0.1.
	AllowNonLoopback bool `json:"allowNonLoopback,omitempty"`
}

// ModelProfile is one model that OpenAI-compatible clients can request.
// Provider backends are shared; several profiles may target the same backend.
//
// Extensibility: advanced fields (reasoning effort, variant, agent,
// temperature, max tokens, context window, system prompt, extra CLI args,
// working dir) are stored with omitempty so they can be adopted later
// without a migration. UpstreamModel/Temperature/MaxTokens/ContextWindow/
// SystemPrompt are wired into requests today; the rest are accepted,
// persisted and exposed so future adapters can use them without a break.
type ModelProfile struct {
	Provider      string     `json:"provider"`
	DisplayName   string     `json:"display_name,omitempty"`
	UpstreamModel string     `json:"upstream_model,omitempty"`
	StreamMode    StreamMode `json:"stream_mode"`
	TimeoutSec    int        `json:"timeout_seconds"`
	Enabled       bool       `json:"enabled"`
	// Future-proof policy fields (accepted + persisted, applied where supported).
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Variant         string   `json:"variant,omitempty"`
	Agent           string   `json:"agent,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       *int     `json:"max_tokens,omitempty"`
	ContextWindow   *int     `json:"context_window,omitempty"`
	SystemPrompt    string   `json:"system_prompt,omitempty"`
	ExtraArgs       []string `json:"extra_args,omitempty"`
	WorkingDir      string   `json:"working_dir,omitempty"`
}

// ProviderConfig holds per-provider runtime tuning. OAuth state is not stored here.
type ProviderConfig struct {
	Enabled         bool `json:"enabled"`
	Concurrency     int  `json:"concurrency"`
	MaxQueue        int  `json:"maxQueue"`
	QueueTimeoutSec int  `json:"queueTimeoutSec"`
	ExecTimeoutSec  int  `json:"execTimeoutSec"`
}

func defaultProvider(alias string, enabled bool) ProviderConfig {
	return ProviderConfig{
		Enabled:         enabled,
		Concurrency:     DefaultConcurrency,
		MaxQueue:        DefaultMaxQueue,
		QueueTimeoutSec: DefaultQueueTimeoutS,
		ExecTimeoutSec:  DefaultExecTimeoutS,
	}
}

var (
	modelIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\-]{0,63}$`)
	// Aliases lists the supported CLI backends in canonical order.
	aliases = []string{"claude", "codex", "gemini", "opencode"}
)

// Config is the full persisted application configuration.
type Config struct {
	Version           int                       `json:"version"`
	Server            ServerConfig              `json:"server"`
	AutoStartServer   bool                      `json:"autoStartServer"`
	RequireAPIKey     bool                      `json:"requireApiKey"`
	APIKey            string                    `json:"apiKey,omitempty"`
	FirstRunDismissed bool                      `json:"firstRunDismissed"`
	SaveLogsToDisk    bool                      `json:"saveLogsToDisk"`
	RetentionDays     int                       `json:"retentionDays"`
	DebugLogging      bool                      `json:"debugLogging"`
	GlobalConcurrency int                       `json:"globalConcurrency"`
	Providers         map[string]ProviderConfig `json:"providers"`
	Models            map[string]ModelProfile   `json:"models"`
	providerFile      string                    `json:"-"`
	mu                sync.RWMutex              `json:"-"`
}

// rawConfig mirrors both v1 and v2 config shapes, so a single decode can feed
// migration. The v1 top-level "port" lives on its own field.
type rawConfig struct {
	Version           int                       `json:"version"`
	Server            ServerConfig              `json:"server"`
	Port              int                       `json:"port"` // v1 legacy top-level port
	AutoStartServer   bool                      `json:"autoStartServer"`
	RequireAPIKey     bool                      `json:"requireApiKey"`
	APIKey            string                    `json:"apiKey,omitempty"`
	FirstRunDismissed bool                      `json:"firstRunDismissed"`
	SaveLogsToDisk    bool                      `json:"saveLogsToDisk"`
	RetentionDays     int                       `json:"retentionDays"`
	DebugLogging      bool                      `json:"debugLogging"`
	GlobalConcurrency int                       `json:"globalConcurrency"`
	Providers         map[string]ProviderConfig `json:"providers"`
	Models            map[string]ModelProfile   `json:"models"`
}

// Default returns a fresh config with sane defaults.
func Default() *Config {
	c := &Config{
		Version:           SchemaVersion,
		Server:            ServerConfig{Host: DefaultHost, Port: DefaultPort},
		AutoStartServer:   true,
		RetentionDays:     DefaultRetentionDays,
		GlobalConcurrency: DefaultGlobalConcurrency,
		Providers:         make(map[string]ProviderConfig),
		Models:            make(map[string]ModelProfile),
	}
	for _, alias := range Aliases() {
		c.Providers[alias] = defaultProvider(alias, true)
	}
	for _, alias := range Aliases() {
		c.Models[alias] = defaultModel(alias, true)
	}
	return c
}

// defaultModel seeds a model profile for one backend provider.
func defaultModel(alias string, enabled bool) ModelProfile {
	sm := StreamDisabled
	switch alias {
	case "claude", "opencode":
		// claude streams tokens natively (stream-json); opencode forwards its
		// text output incrementally.
		sm = StreamNative
	}
	return ModelProfile{
		Provider:   alias,
		StreamMode: sm,
		TimeoutSec: DefaultModelTimeout,
		Enabled:    enabled,
	}
}

// AppDataDir returns %APPDATA%\LocalAIProxy, creating it if needed.
func AppDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "LocalAIProxy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// FromFile loads Config from %APPDATA%\LocalAIProxy\config.json.
// Missing or corrupt files fall back to defaults (with the file being rewritten on next save).
func FromFile() (*Config, error) {
	dir, err := AppDataDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	return LoadFile(path)
}

// LoadFile reads a config from an explicit path. Used by tests.
func LoadFile(path string) (*Config, error) {
	c := Default()
	c.providerFile = path
	b, err := os.ReadFile(path)
	if err != nil {
		// Missing or unreadable => defaults; never crash the app on config.
		return c, nil
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return c, nil // corrupt => defaults
	}
	applyRaw(c, &raw)
	return c, nil
}

// applyRaw merges a decoded config file (old or new shape) into defaults and
// runs the v1 -> v2 migration when needed.
func applyRaw(c *Config, raw *rawConfig) {
	// server — loopback safety: non-loopback hosts require explicit opt-in.
	if raw.Server.Host != "" {
		c.Server.Host = raw.Server.Host
	}
	c.Server.AllowNonLoopback = raw.Server.AllowNonLoopback
	if !IsLoopbackHost(c.Server.Host) && !c.Server.AllowNonLoopback {
		c.Server.Host = DefaultHost
	}
	switch {
	case raw.Server.Port >= 1 && raw.Server.Port <= 65535:
		c.Server.Port = raw.Server.Port
	case raw.Port >= 1 && raw.Port <= 65535:
		c.Server.Port = raw.Port // migrated from v1 top-level port
	default:
		c.Server.Port = DefaultPort
	}

	c.AutoStartServer = raw.AutoStartServer
	c.RequireAPIKey = raw.RequireAPIKey
	c.APIKey = raw.APIKey
	c.FirstRunDismissed = raw.FirstRunDismissed
	c.SaveLogsToDisk = raw.SaveLogsToDisk
	if raw.RetentionDays > 0 {
		c.RetentionDays = raw.RetentionDays
	}
	c.DebugLogging = raw.DebugLogging
	if raw.GlobalConcurrency >= 1 && raw.GlobalConcurrency <= MaxGlobalConcurrency {
		c.GlobalConcurrency = raw.GlobalConcurrency
	} else {
		c.GlobalConcurrency = DefaultGlobalConcurrency
	}

	if raw.Providers != nil {
		for alias, p := range raw.Providers {
			c.Providers[alias] = normalizeProvider(p)
		}
	}

	// models: v2 files carry their own profiles (an explicit empty map "{}" is
	// respected as an intentional empty registry). A nil map means this is a v1
	// file — seed one profile per provider so existing clients using
	// model="claude" etc. keep working.
	if raw.Models == nil {
		for _, alias := range Aliases() {
			c.Models[alias] = defaultModel(alias, c.Provider(alias).Enabled)
		}
	} else {
		c.Models = make(map[string]ModelProfile)
		for id, m := range raw.Models {
			c.Models[id] = normalizeModel(m)
		}
	}
}

func normalizeProvider(p ProviderConfig) ProviderConfig {
	if p.Concurrency < 1 {
		p.Concurrency = DefaultConcurrency
	}
	if p.MaxQueue < 0 {
		p.MaxQueue = DefaultMaxQueue
	}
	if p.QueueTimeoutSec < 0 {
		p.QueueTimeoutSec = DefaultQueueTimeoutS
	}
	if p.ExecTimeoutSec < 0 {
		p.ExecTimeoutSec = DefaultExecTimeoutS
	}
	return p
}

func normalizeModel(m ModelProfile) ModelProfile {
	m.StreamMode = ParseStreamMode(string(m.StreamMode))
	if m.TimeoutSec < 1 {
		m.TimeoutSec = DefaultModelTimeout
	}
	if m.TimeoutSec > MaxTimeoutSec {
		m.TimeoutSec = MaxTimeoutSec
	}
	m.UpstreamModel = strings.TrimSpace(m.UpstreamModel)
	if strings.EqualFold(m.UpstreamModel, "default") {
		m.UpstreamModel = ""
	}
	if m.Temperature != nil && (*m.Temperature < 0 || *m.Temperature > 2) {
		m.Temperature = nil
	}
	if m.MaxTokens != nil && *m.MaxTokens < 1 {
		m.MaxTokens = nil
	}
	if m.ContextWindow != nil && *m.ContextWindow < 1 {
		m.ContextWindow = nil
	}
	return m
}

// Aliases returns the supported provider backend aliases in canonical order.
func Aliases() []string { return append([]string(nil), aliases...) }

// ValidateModelID checks that id is a safe OpenAI-compatible model identifier.
// Empty id is reported; the UI should never pass one.
func ValidateModelID(id string) error {
	if id == "" {
		return fmt.Errorf("model id is required")
	}
	if len(id) > 64 || !modelIDRe.MatchString(id) {
		return fmt.Errorf("model id must start with a letter or digit and contain only letters, digits, '.', '_', ':', '-'")
	}
	return nil
}

// Path returns the config file path.
func (c *Config) Path() string {
	if c.providerFile != "" {
		return c.providerFile
	}
	dir, _ := AppDataDir()
	return filepath.Join(dir, "config.json")
}

// Save writes the config atomically: tmp file + rename.
func (c *Config) Save() error {
	c.mu.RLock()
	b, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	path := c.Path()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Provider returns a copy of the provider config for alias.
func (c *Config) Provider(alias string) ProviderConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.Providers[alias]; ok {
		return p
	}
	return defaultProvider(alias, true)
}

// SetProvider updates a provider config and persists.
func (c *Config) SetProvider(alias string, p ProviderConfig) error {
	c.mu.Lock()
	c.Providers[alias] = p
	c.mu.Unlock()
	return c.Save()
}

// Model returns a copy of the model profile for id and whether it exists.
func (c *Config) Model(id string) (ModelProfile, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.Models[id]
	return m, ok
}

// SetModel upserts a model profile and persists.
func (c *Config) SetModel(id string, m ModelProfile) error {
	if err := ValidateModelID(id); err != nil {
		return err
	}
	c.mu.Lock()
	c.Models[id] = normalizeModel(m)
	c.mu.Unlock()
	return c.Save()
}

// DeleteModel removes a model profile and persists. Deleting a missing id is
// a no-op (success) so the UI can delete idempotently.
func (c *Config) DeleteModel(id string) error {
	c.mu.Lock()
	delete(c.Models, id)
	c.mu.Unlock()
	return c.Save()
}

// ModelIDs returns all model ids in stable sorted order.
func (c *Config) ModelIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make([]string, 0, len(c.Models))
	for id := range c.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ValidatePort returns an error if the server port is unusable.
func (c *Config) ValidatePort() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// IsLoopbackHost reports whether host is a loopback address.
func IsLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	switch h {
	case "", "127.0.0.1", "localhost", "::1", "::ffff:127.0.0.1":
		return true
	}
	if strings.HasPrefix(h, "127.") {
		return true
	}
	return false
}

// ValidateHost enforces loopback-only binding unless explicitly allowed.
func (c *Config) ValidateHost() error {
	if IsLoopbackHost(c.Server.Host) {
		return nil
	}
	if c.Server.AllowNonLoopback {
		return nil
	}
	return fmt.Errorf("non-loopback host %q requires explicit allowNonLoopback opt-in", c.Server.Host)
}
