package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Server.Port != DefaultPort {
		t.Fatalf("port = %d, want %d", c.Server.Port, DefaultPort)
	}
	if c.Server.Host != DefaultHost {
		t.Fatalf("host = %q", c.Server.Host)
	}
	if !c.AutoStartServer {
		t.Fatal("autostart should default to true")
	}
	if c.RetentionDays != DefaultRetentionDays {
		t.Fatalf("retention = %d", c.RetentionDays)
	}
	for _, alias := range Aliases() {
		p := c.Provider(alias)
		if !p.Enabled {
			t.Errorf("%s not enabled by default", alias)
		}
		if p.Concurrency != DefaultConcurrency {
			t.Errorf("%s concurrency = %d", alias, p.Concurrency)
		}
	}
	// Default model profiles seeded with honest stream modes.
	if m, ok := c.Model("claude"); !ok || m.StreamMode != StreamNative {
		t.Errorf("claude model = %+v, want stream=native", m)
	}
	if m, ok := c.Model("codex"); !ok || m.StreamMode != StreamDisabled {
		t.Errorf("codex model = %+v, want stream=disabled", m)
	}
	if m, ok := c.Model("opencode"); !ok || m.StreamMode != StreamNative {
		t.Errorf("opencode model = %+v, want stream=native", m)
	}
}

func TestAliasOrder(t *testing.T) {
	want := []string{"claude", "codex", "gemini", "opencode"}
	got := Aliases()
	if len(got) != len(want) {
		t.Fatalf("aliases = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aliases[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := LoadFile(path) // binds c to the temp path like production does
	if err != nil {
		t.Fatal(err)
	}
	c.Server.Port = 9001
	c.AutoStartServer = false
	c.RequireAPIKey = true
	c.APIKey = "sekret"
	c.RetentionDays = 3
	pc := c.Provider("codex")
	pc.Enabled = false
	pc.Concurrency = 4
	if err := c.SetProvider("codex", pc); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.Port != 9001 || got.AutoStartServer || !got.RequireAPIKey || got.APIKey != "sekret" {
		t.Fatalf("roundtrip failed: %+v", got)
	}
	if got.RetentionDays != 3 {
		t.Fatalf("retention = %d", got.RetentionDays)
	}
	p := got.Provider("codex")
	if p.Enabled || p.Concurrency != 4 {
		t.Fatalf("codex roundtrip failed: %+v", p)
	}
}

func TestLoadMissingFallsBackToDefaults(t *testing.T) {
	c, err := LoadFile(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Port != DefaultPort {
		t.Fatalf("port = %d", c.Server.Port)
	}
}

func TestLoadCorruptFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Port != DefaultPort || c.RetentionDays != DefaultRetentionDays {
		t.Fatalf("expected sanitized defaults, got %+v", c)
	}
}

func TestLoadPartialDefaultsFilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Only claude present, port omitted.
	if err := os.WriteFile(path, []byte(`{"providers":{"claude":{"enabled":false,"concurrency":3}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Port != DefaultPort {
		t.Fatalf("port = %d", c.Server.Port)
	}
	if c.Provider("claude").Concurrency != 3 {
		t.Fatalf("claude concurrency = %d", c.Provider("claude").Concurrency)
	}
	for _, alias := range []string{"codex", "gemini", "opencode"} {
		if !c.Provider(alias).Enabled {
			t.Errorf("%s should default to enabled", alias)
		}
	}
}

func TestProviderNumbersClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"providers":{"claude":{"enabled":true,"concurrency":0,"maxQueue":-2,"queueTimeoutSec":-1}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Provider("claude")
	if p.Concurrency != DefaultConcurrency || p.MaxQueue != DefaultMaxQueue || p.QueueTimeoutSec != DefaultQueueTimeoutS {
		t.Fatalf("clamps failed: %+v", p)
	}
}

func TestV1Migration(t *testing.T) {
	// A v1 file: top-level port, providers only (no models array means "v1").
	path := filepath.Join(t.TempDir(), "config.json")
	in := `{"version":1,"port":9000,"providers":{"codex":{"enabled":true}},"models":{}}`
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Version-normalized to 2; port migrated under server.
	if c.Server.Port != 9000 {
		t.Fatalf("migrated port = %d", c.Server.Port)
	}
	// Explicit empty models object is respected: no seeded profiles.
	if got := len(c.Models); got != 0 {
		t.Fatalf("models = %d, want 0 (explicit empty map kept)", got)
	}
}

func TestV1ModelSeeding(t *testing.T) {
	// True v1 shape: no "models" key at all -> seed default profiles.
	path := filepath.Join(t.TempDir(), "config.json")
	in := `{"port":9000,"providers":{"claude":{"enabled":false}}}`
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Port != 9000 {
		t.Fatalf("port = %d", c.Server.Port)
	}
	if got := len(c.Models); got != len(Aliases()) {
		t.Fatalf("seeded %d models, want %d", got, len(Aliases()))
	}
	// claude profile mirrors the disabled provider.
	if m, ok := c.Model("claude"); !ok {
		t.Fatal("claude profile missing")
	} else if m.Enabled {
		t.Fatal("claude profile should inherit provider disabled state")
	}
}

func TestModelNormalizeAndClamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	in := `{"models":{"weird":{"provider":"claude","stream_mode":"BOGUS","timeout_seconds":0}}}`
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Model("weird")
	if !ok {
		t.Fatal("model missing")
	}
	if m.StreamMode != StreamDisabled {
		t.Fatalf("bad stream mode should clamp to disabled, got %q", m.StreamMode)
	}
	if m.TimeoutSec != DefaultModelTimeout {
		t.Fatalf("timeout = %d, want %d", m.TimeoutSec, DefaultModelTimeout)
	}
}

func TestValidateModelID(t *testing.T) {
	valid := []string{"claude", "claude-sonnet-4", "my.model:v1", "x_y-1", "A"}
	for _, id := range valid {
		if err := ValidateModelID(id); err != nil {
			t.Errorf("%q should be valid: %v", id, err)
		}
	}
	invalid := []string{"", "-a", "a b", "a/b", "😀", "a b c", ">echo", strings.Repeat("a", 65)}
	for _, id := range invalid {
		if err := ValidateModelID(id); err == nil {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestSetDeleteModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetModel("my-model", ModelProfile{Provider: "gemini", StreamMode: StreamNative, TimeoutSec: 42, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Model("my-model"); !ok || got.TimeoutSec != 42 || got.StreamMode != StreamNative {
		t.Fatalf("model = %+v", got)
	}
	if err := c.DeleteModel("my-model"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Model("my-model"); ok {
		t.Fatal("model should be deleted")
	}
}