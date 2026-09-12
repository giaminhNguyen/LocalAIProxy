// Package config persists LocalAIProxy application settings.
//
// Settings live in %APPDATA%\LocalAIProxy\config.json.
// Provider OAuth credentials are NEVER stored here - the CLIs own their own auth.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultPort        = 8317
	DefaultConcurrency = 1
	DefaultMaxQueue    = 10
	// DefaultQueueTimeoutSec 0 = wait indefinitely while queued.
	DefaultQueueTimeoutSec = 120
	// DefaultExecTimeoutSec 0 = Unlimited execution time.
	DefaultExecTimeoutSec = 0
	DefaultRetentionDays  = 7
)

// ProviderConfig holds per-provider tuning. OAuth state is not stored here.
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
		QueueTimeoutSec: DefaultQueueTimeoutSec,
		ExecTimeoutSec:  DefaultExecTimeoutSec,
	}
}

// Config is the full persisted application configuration.
type Config struct {
	Port              int                       `json:"port"`
	AutoStartServer   bool                      `json:"autoStartServer"`
	RequireAPIKey     bool                      `json:"requireApiKey"`
	APIKey            string                    `json:"apiKey,omitempty"`
	FirstRunDismissed bool                      `json:"firstRunDismissed"`
	SaveLogsToDisk    bool                      `json:"saveLogsToDisk"`
	RetentionDays     int                       `json:"retentionDays"`
	DebugLogging      bool                      `json:"debugLogging"`
	Providers         map[string]ProviderConfig `json:"providers"`
	providerFile      string                    `json:"-"`
	mu                sync.RWMutex              `json:"-"`
}

// Default returns a fresh config with sane defaults.
func Default() *Config {
	c := &Config{
		Port:            DefaultPort,
		AutoStartServer: true,
		RetentionDays:   DefaultRetentionDays,
		Providers:       make(map[string]ProviderConfig),
	}
	c.Providers["claude"] = defaultProvider("claude", true)
	c.Providers["codex"] = defaultProvider("codex", true)
	c.Providers["gemini"] = defaultProvider("gemini", true)
	c.Providers["opencode"] = defaultProvider("opencode", true)
	return c
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
	def := Default()
	def.providerFile = path
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return def, nil
		}
		return def, nil // unreadable => defaults; never crash the app on config
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return def, nil
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = DefaultPort
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = DefaultRetentionDays
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	for _, alias := range Aliases() {
		p, ok := c.Providers[alias]
		if !ok {
			c.Providers[alias] = defaultProvider(alias, true)
			continue
		}
		if p.Concurrency < 1 {
			p.Concurrency = DefaultConcurrency
		}
		if p.MaxQueue < 0 {
			p.MaxQueue = DefaultMaxQueue
		}
		if p.QueueTimeoutSec < 0 {
			p.QueueTimeoutSec = DefaultQueueTimeoutSec
		}
		if p.ExecTimeoutSec < 0 {
			p.ExecTimeoutSec = DefaultExecTimeoutSec
		}
		c.Providers[alias] = p
	}
	c.providerFile = path
	return &c, nil
}

// Aliases returns the four fixed provider aliases in canonical order.
func Aliases() []string { return []string{"claude", "codex", "gemini", "opencode"} }

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

// ValidatePort returns an error if the port is unusable.
func (c *Config) ValidatePort() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}
