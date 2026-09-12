package core

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"LocalAIProxy/internal/config"
	"LocalAIProxy/internal/provider"
)

// TestProviderLevelScheduling ensures two models on the same provider share one queue.
func TestProviderLevelScheduling(t *testing.T) {
	cfg := freshConfig(t)
	if err := cfg.SetModel("model-a", config.ModelProfile{Provider: "codex", TimeoutSec: 30, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetModel("model-b", config.ModelProfile{Provider: "codex", TimeoutSec: 30, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	fk := &fakeRunner{result: provider.Result{Content: "ok"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)
	c.resetGlobalSemLocked(10)

	qa := c.queueFor("codex", config.ModelProfile{Provider: "codex"})
	qb := c.queueFor("codex", config.ModelProfile{Provider: "codex"})
	if qa != qb {
		t.Fatal("models on the same provider must share one scheduler")
	}
	// Different providers get different schedulers.
	qc := c.queueFor("gemini", config.ModelProfile{Provider: "gemini"})
	if qc == qa {
		t.Fatal("different providers must not share a scheduler")
	}
}

// TestGlobalConcurrencyOne ensures global=1 serializes across providers.
func TestGlobalConcurrencyOne(t *testing.T) {
	cfg := freshConfig(t)
	fk := &fakeRunner{result: provider.Result{Content: "ok"}}
	c := testCore(t, cfg, defaultTestInfos(), fk)
	c.resetGlobalSemLocked(1)

	var max int64
	var cur int64
	release1, err1 := c.acquireGlobal(context.Background())
	if err1 != nil {
		t.Fatal(err1)
	}
	atomic.AddInt64(&cur, 1)
	max = 1
	done := make(chan struct{})
	go func() {
		rel, err := c.acquireGlobal(context.Background())
		if err != nil {
			close(done)
			return
		}
		n := atomic.AddInt64(&cur, 1)
		for {
			m := atomic.LoadInt64(&max)
			if n <= m || atomic.CompareAndSwapInt64(&max, m, n) {
				break
			}
		}
		atomic.AddInt64(&cur, -1)
		rel()
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	// Second acquirer must still be waiting (global=1 held).
	select {
	case <-done:
		t.Fatal("global concurrency=1 violated: second slot acquired while first held")
	default:
	}
	atomic.AddInt64(&cur, -1)
	release1()
	<-done
	if atomic.LoadInt64(&max) > 1 {
		t.Fatalf("max concurrent = %d, want <=1", max)
	}
}

// TestAPIKeyCaseSensitive ensures exact constant-time comparison.
func TestAPIKeyCaseSensitive(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	cfg.RequireAPIKey = true
	cfg.APIKey = "lap-AbC123"
	if !c.ValidAPIKey("lap-AbC123") {
		t.Fatal("exact key must validate")
	}
	if c.ValidAPIKey("lap-abc123") {
		t.Fatal("different case must NOT validate")
	}
	if c.ValidAPIKey("") || c.ValidAPIKey("wrong") {
		t.Fatal("empty/wrong must NOT validate")
	}
}

// TestLoopbackEnforcement ensures non-loopback hosts are reset without opt-in.
func TestLoopbackEnforcement(t *testing.T) {
	if !config.IsLoopbackHost("127.0.0.1") || !config.IsLoopbackHost("localhost") || !config.IsLoopbackHost("::1") {
		t.Fatal("loopback detection broken")
	}
	if config.IsLoopbackHost("0.0.0.0") || config.IsLoopbackHost("192.168.1.2") {
		t.Fatal("0.0.0.0 must not count as loopback")
	}
	c := config.Default()
	c.Server.Host = "0.0.0.0"
	if err := c.ValidateHost(); err == nil {
		t.Fatal("non-loopback without opt-in must fail validation")
	}
	c.Server.AllowNonLoopback = true
	if err := c.ValidateHost(); err != nil {
		t.Fatalf("opt-in non-loopback should validate: %v", err)
	}
}

// TestGlobalConcurrencyValidation checks bounds.
func TestGlobalConcurrencyValidation(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	if err := c.SetGlobalConcurrency(0); err == nil {
		t.Fatal("0 must be rejected")
	}
	if err := c.SetGlobalConcurrency(4); err != nil {
		t.Fatal(err)
	}
	if cfg.GlobalConcurrency != 4 {
		t.Fatalf("global = %d", cfg.GlobalConcurrency)
	}
}

// TestDuplicateModel checks copy behavior.
func TestDuplicateModel(t *testing.T) {
	cfg := freshConfig(t)
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	if err := c.DuplicateModel("codex", "codex-copy"); err != nil {
		t.Fatal(err)
	}
	src, _ := cfg.Model("codex")
	dst, ok := cfg.Model("codex-copy")
	if !ok || dst.Provider != src.Provider {
		t.Fatalf("dup = %+v", dst)
	}
}

// TestSnapshotCarriesAPIKeyHint ensures the dashboard auth hint has data.
func TestSnapshotCarriesAPIKeyHint(t *testing.T) {
	cfg := freshConfig(t)
	cfg.RequireAPIKey = true
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	snap := c.Snapshot()
	if !snap.RequireAPIKey {
		t.Fatal("snapshot must carry requireApiKey for the dashboard hint")
	}
}

// TestConfigureKeepsPortOnOccupied ensures Settings port apply never diverges.
func TestConfigureKeepsPortOnOccupied(t *testing.T) {
	cfg := freshConfig(t)
	old := cfg.Server.Port
	c := testCore(t, cfg, defaultTestInfos(), &fakeRunner{})
	// Occupy a port, then try to Configure onto it: must fail and keep old.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind test port")
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	if err := c.Configure(map[string]any{"port": float64(busy)}); err == nil {
		t.Fatal("occupied port must be rejected")
	}
	if cfg.Server.Port != old {
		t.Fatalf("port diverged: got %d want %d", cfg.Server.Port, old)
	}
}
