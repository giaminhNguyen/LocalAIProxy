package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"LocalAIProxy/internal/provider"
)

// blockingRunner blocks until unblock ch is closed; when closed it returns a
// provider_timeout error so tests never hang.
type blockingRunner struct {
	unblock chan struct{}
	started chan struct{}
	once    sync.Once
	active  int64
	max     int64
}

func (r *blockingRunner) Run(ctx context.Context, inv provider.Invocation) (provider.Result, error) {
	n := atomic.AddInt64(&r.active, 1)
	defer atomic.AddInt64(&r.active, -1)
	for {
		cur := atomic.LoadInt64(&r.active)
		old := atomic.LoadInt64(&r.max)
		if cur > old {
			atomic.CompareAndSwapInt64(&r.max, old, cur)
		}
		break
	}
	if n == 1 {
		// first runner signals it entered Run
		r.once.Do(func() { close(r.started) })
	}
	select {
	case <-r.unblock:
		return provider.Result{Content: "ok"}, nil
	case <-ctx.Done():
		return provider.Result{}, provider.NewError(provider.ErrProviderTimeout, "codex", "timed out", 504)
	}
}

func release(r *blockingRunner) { close(r.unblock) }

func TestQueueRuns(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 1, MaxQueue: 5}, br)

	done := make(chan struct{})
	var res provider.Result
	var err error
	go func() {
		res, err = q.Submit(context.Background(), provider.Invocation{})
		close(done)
	}()
	<-br.started
	release(br)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "ok" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestQueueBusyWhenGateFull(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 1, MaxQueue: 1}, br)

	// First request occupies the single gate slot.
	done := make(chan struct{})
	go func() {
		_, _ = q.Submit(context.Background(), provider.Invocation{})
		close(done)
	}()
	<-br.started

	// Second request: gate full -> busy immediately.
	_, err := q.Submit(context.Background(), provider.Invocation{})
	if perr, ok := errAsProviderError(t, err); !ok {
		t.Fatal("expected provider error")
	} else if perr.Code != provider.ErrProviderBusy {
		t.Fatalf("code = %q, want %q", perr.Code, provider.ErrProviderBusy)
	}

	release(br)
	<-done
}

func TestQueueTimeoutWhileWaiting(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 1, MaxQueue: 5, QueueTimeout: 40 * time.Millisecond}, br)

	done := make(chan struct{})
	go func() {
		_, _ = q.Submit(context.Background(), provider.Invocation{})
		close(done)
	}()
	<-br.started

	start := time.Now()
	_, err := q.Submit(context.Background(), provider.Invocation{})
	if perr, ok := errAsProviderError(t, err); !ok {
		t.Fatal("expected provider error")
	} else if perr.Code != provider.ErrQueueTimeout {
		t.Fatalf("code = %q, want %q", perr.Code, provider.ErrQueueTimeout)
	}
	if elapsed := time.Since(start); elapsed < 35*time.Millisecond {
		t.Fatalf("returned too early: %v", elapsed)
	}

	release(br)
	<-done
}

func TestExecTimeoutCancelsRunner(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 1, MaxQueue: 5, ExecTimeout: 40 * time.Millisecond}, br)

	_, err := q.Submit(context.Background(), provider.Invocation{})
	if perr, ok := errAsProviderError(t, err); !ok {
		t.Fatal("expected provider error")
	} else if perr.Code != provider.ErrProviderTimeout {
		t.Fatalf("code = %q, want %q", perr.Code, provider.ErrProviderTimeout)
	}
}

func TestConcurrencyCapped(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 2, MaxQueue: 20}, br)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = q.Submit(context.Background(), provider.Invocation{})
	}()
	<-br.started

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = q.Submit(context.Background(), provider.Invocation{})
		}()
	}
	release(br) // free the initial slot so queued requests can proceed
	wg.Wait()
	<-done

	if br.max > 2 {
		t.Fatalf("max concurrency = %d, want <= 2", br.max)
	}
}

func TestQueueTimeoutZeroWaitsIndefinitely(t *testing.T) {
	br := &blockingRunner{unblock: make(chan struct{}), started: make(chan struct{})}
	q := New("codex", Config{Concurrency: 1, MaxQueue: 5, QueueTimeout: 0}, br)

	done := make(chan struct{})
	go func() {
		_, _ = q.Submit(context.Background(), provider.Invocation{})
		close(done)
	}()
	<-br.started

	finished := make(chan struct{})
	go func() {
		_, _ = q.Submit(context.Background(), provider.Invocation{})
		close(finished)
	}()

	time.Sleep(30 * time.Millisecond)
	select {
	case <-finished:
		t.Fatal("request returned while queue timeout is zero")
	default:
	}
	release(br)
	<-done
	<-finished
}

func errAsProviderError(t *testing.T, err error) (provider.Error, bool) {
	t.Helper()
	if err == nil {
		return provider.Error{}, false
	}
	var pe *provider.Error
	if provider.AsError(err, &pe) {
		return *pe, true
	}
	t.Fatalf("error is not a *provider.Error: %T %v", err, err)
	return provider.Error{}, false
}
