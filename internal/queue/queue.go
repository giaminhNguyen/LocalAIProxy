// Package queue provides per-provider concurrency limiting with queueing.
//
// Model: one CLI process per HTTP request. A semaphore caps simultaneous
// executions; a buffered gate caps total queued+active requests. Queue
// timeout applies ONLY while waiting for a free slot — it never kills a
// request that already started. Execution timeout is applied around Run.
package queue

import (
	"context"
	"fmt"
	"time"

	"LocalAIProxy/internal/provider"
)

// Config tunes one provider queue.
type Config struct {
	Concurrency  int
	MaxQueue     int
	QueueTimeout time.Duration // 0 = wait indefinitely while queued
	ExecTimeout  time.Duration // 0 = unlimited execution
}

// Queue serializes inbound requests to a single provider. Queues are rebuilt
// (New) whenever settings change, so they are read-only after construction.
type Queue struct {
	name   string
	cfg    Config
	runner provider.Runner
	gate   chan struct{} // tickets for queued+active (nil when unbounded)
	slots  chan struct{} // tickets for active executions
}

// New builds a queue from config.
func New(name string, cfg Config, runner provider.Runner) *Queue {
	q := &Queue{name: name, cfg: cfg, runner: runner}
	if cfg.MaxQueue > 0 {
		q.gate = make(chan struct{}, cfg.MaxQueue)
	}
	conc := cfg.Concurrency
	if conc < 1 {
		conc = 1
	}
	q.slots = make(chan struct{}, conc)
	return q
}

// Submit runs inv, waiting in the queue as needed. Returns normalized
// provider errors: provider_busy (gate full), queue_timeout, provider_timeout.
func (q *Queue) Submit(ctx context.Context, inv provider.Invocation) (provider.Result, error) {
	waitStart := time.Now()

	// Gate: bounded queue. If full -> provider_busy.
	if q.gate != nil {
		select {
		case q.gate <- struct{}{}:
		case <-ctx.Done():
			return provider.Result{}, *provider.NewError(provider.ErrRequestCancelled, q.name, "Request cancelled by the client.", 499)
		default:
			return provider.Result{}, *provider.NewError(
				provider.ErrProviderBusy, q.name,
				fmt.Sprintf("%s is busy — too many requests are queued. Try again in a moment.", q.name), 429,
			)
		}
		defer func() { <-q.gate }()
	}

	// Slot: wait for a free execution slot, bounded by queue timeout.
	select {
	case q.slots <- struct{}{}:
	case <-ctx.Done():
		return provider.Result{}, *provider.NewError(provider.ErrRequestCancelled, q.name, "Request cancelled by the client.", 499)
	case <-afterQueueTimeout(q.cfg.QueueTimeout, waitStart):
		return provider.Result{}, *provider.NewError(
			provider.ErrQueueTimeout, q.name,
			"The request waited in the queue too long and was cancelled.", 504,
		)
	}
	defer func() { <-q.slots }()

	// Execution timeout covers only the running phase.
	runCtx := ctx
	var cancel context.CancelFunc
	if q.cfg.ExecTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, q.cfg.ExecTimeout)
		defer cancel()
	}

	return q.runner.Run(runCtx, inv)
}

func afterQueueTimeout(d time.Duration, start time.Time) <-chan time.Time {
	if d <= 0 {
		return nil // wait indefinitely
	}
	remain := d - time.Since(start)
	if remain <= 0 {
		remain = time.Millisecond
	}
	return time.After(remain)
}
