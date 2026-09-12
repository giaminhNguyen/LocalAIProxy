package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Port != DefaultPort {
		t.Fatalf("port = %d, want %d", c.Port, DefaultPort)
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
	c.Port = 9001
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
	if got.Port != 9001 || got.AutoStartServer || !got.RequireAPIKey || got.APIKey != "sekret" {
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
	if c.Port != DefaultPort {
		t.Fatalf("port = %d", c.Port)
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
	if c.Port != DefaultPort || c.RetentionDays != DefaultRetentionDays {
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
	if c.Port != DefaultPort {
		t.Fatalf("port = %d", c.Port)
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
	if p.Concurrency != DefaultConcurrency || p.MaxQueue != DefaultMaxQueue || p.QueueTimeoutSec != DefaultQueueTimeoutSec {
		t.Fatalf("clamps failed: %+v", p)
	}
}
