// Package scheduler is a facade over the hierarchical timing wheel. It assigns
// stable ids, exposes a simple Schedule/Cancel/Shutdown API, runs a clock-driven
// loop, tracks metrics, and propagates context cancellation on shutdown.
//
// The scheduler is deterministic with respect to its Clock: when given a fake
// clock it fires tasks only when that clock is advanced, which makes every
// behaviour here unit-testable without sleeping on the wall clock.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"timer-wheel/internal/clock"
	"timer-wheel/internal/wheel"
)

// Metrics reports cumulative counters for the scheduler's lifetime.
type Metrics struct {
	// Scheduled is the number of successful Schedule calls.
	Scheduled int64
	// Fired is the number of tasks whose callback actually ran.
	Fired int64
	// Cancelled is the number of tasks removed before they fired, whether by an
	// explicit Cancel or by Shutdown.
	Cancelled int64
}

// Scheduler coordinates task scheduling on top of a hierarchical timing wheel.
//
// The zero value is not usable; construct with New.
type Scheduler struct {
	mu       sync.Mutex
	clock    clock.Clock
	wheel    *wheel.Wheel
	seq      int64
	started  bool
	shutdown bool
	baseCtx  context.Context
	cancel   context.CancelFunc
	wake     chan struct{}

	scheduled int64
	fired     int64
	cancelled int64
}

// Options configures a Scheduler.
type Options struct {
	// Tick is the finest granularity of the underlying wheel. Defaults to 1ms.
	Tick time.Duration
	// WheelSize is the number of buckets per wheel level. Defaults to 20.
	WheelSize int
}

// New builds a Scheduler backed by the given clock. The clock must be provided:
// a fake clock yields deterministic tests, a real clock yields a live service.
func New(c clock.Clock, opts Options) *Scheduler {
	tick := opts.Tick
	if tick <= 0 {
		tick = time.Millisecond
	}
	ws := opts.WheelSize
	if ws <= 0 {
		ws = 20
	}
	return &Scheduler{
		clock: c,
		wheel: wheel.New(wheel.Options{Tick: tick, WheelSize: ws, Start: time.Unix(0, 0).UTC()}),
		wake:  make(chan struct{}, 1),
	}
}

// ScheduleOptions customizes a single ScheduleWith call.
type ScheduleOptions struct {
	// Priority orders tasks that become due at the same instant: a higher
	// priority fires first, and equal priorities are broken by ascending id.
	// Zero (the default) is fine for most callers.
	Priority int
	// Label is an optional, free-form tag attached to the task for
	// introspection via Stats; it never affects scheduling behaviour.
	Label string
}

// Schedule registers fn to run after delay. It returns a stable id that can be
// passed to Cancel. Negative delays and nil callbacks are rejected with an
// error rather than silently misfiring.
func (s *Scheduler) Schedule(delay time.Duration, fn func()) (int64, error) {
	return s.ScheduleWith(delay, ScheduleOptions{}, fn)
}

// ScheduleWith is the configurable form of Schedule. It accepts scheduling
// options (currently priority and an introspection label) and otherwise behaves
// identically to Schedule.
func (s *Scheduler) ScheduleWith(delay time.Duration, opts ScheduleOptions, fn func()) (int64, error) {
	if delay < 0 {
		return 0, fmt.Errorf("timer-wheel: negative delay %s", delay)
	}
	if fn == nil {
		return 0, errors.New("timer-wheel: nil callback")
	}
	s.mu.Lock()
	s.seq++
	id := s.seq
	s.mu.Unlock()

	s.wheel.Add(id, delay, opts.Priority, s.wrap(id, fn))
	atomic.AddInt64(&s.scheduled, 1)
	s.signalWake()
	return id, nil
}

// wrap decorates a user callback with metric accounting and cancellation
// awareness so a task cancelled during shutdown is counted but not run.
func (s *Scheduler) wrap(_ int64, fn func()) func() {
	return func() {
		if s.isShutdown() {
			atomic.AddInt64(&s.cancelled, 1)
			return
		}
		if s.baseCtx != nil && s.baseCtx.Err() != nil {
			atomic.AddInt64(&s.cancelled, 1)
			return
		}
		atomic.AddInt64(&s.fired, 1)
		fn()
	}
}

// Cancel removes a scheduled task before it fires. It reports whether a live
// task with that id was actually cancelled.
func (s *Scheduler) Cancel(id int64) bool {
	ok := s.wheel.Cancel(id)
	if ok {
		atomic.AddInt64(&s.cancelled, 1)
	}
	return ok
}

// Shutdown cancels every pending task, updates metrics accordingly, and cancels
// the scheduler's base context so any dependent work observes cancellation.
// Calling Shutdown more than once is safe and idempotent.
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return
	}
	s.shutdown = true
	cancel := s.cancel
	s.mu.Unlock()

	var cancelled int64
	for _, id := range s.wheel.PendingIDs() {
		if s.wheel.Cancel(id) {
			cancelled++
		}
	}
	if cancelled > 0 {
		atomic.AddInt64(&s.cancelled, cancelled)
	}
	if cancel != nil {
		cancel()
	}
}

// Metrics returns a snapshot of the cumulative counters.
func (s *Scheduler) Metrics() Metrics {
	return Metrics{
		Scheduled: atomic.LoadInt64(&s.scheduled),
		Fired:     atomic.LoadInt64(&s.fired),
		Cancelled: atomic.LoadInt64(&s.cancelled),
	}
}

// Stats is a point-in-time operational snapshot of the scheduler. Unlike
// Metrics, which only reports cumulative counters, Stats also exposes the live
// queue depth and the next due instant, so a caller can introspect how busy the
// scheduler is right now.
type Stats struct {
	// Pending is the number of tasks currently scheduled but not yet fired.
	Pending int
	// Scheduled, Fired, Cancelled mirror the cumulative Metrics counters.
	Scheduled int64
	Fired     int64
	Cancelled int64
	// NextDeadline is the earliest expiration among pending tasks, or the zero
	// time if nothing is pending.
	NextDeadline time.Time
	// Running reports whether Start has been called and Shutdown has not.
	Running bool
}

// Stats returns a snapshot of the scheduler's current operational state. It is
// safe to call at any time, including concurrently with scheduling activity.
func (s *Scheduler) Stats() Stats {
	s.mu.Lock()
	running := s.started && !s.shutdown
	s.mu.Unlock()
	return Stats{
		Pending:      s.wheel.Len(),
		Scheduled:    atomic.LoadInt64(&s.scheduled),
		Fired:        atomic.LoadInt64(&s.fired),
		Cancelled:    atomic.LoadInt64(&s.cancelled),
		NextDeadline: s.wheel.NextDeadline(),
		Running:      running,
	}
}

// Done returns the scheduler's base-context done channel, or nil if the
// scheduler has not been started. After Shutdown the channel is closed.
func (s *Scheduler) Done() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseCtx == nil {
		return nil
	}
	return s.baseCtx.Done()
}

// isShutdown reports whether Shutdown has been called.
func (s *Scheduler) isShutdown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdown
}

// signalWake nudges the real-clock run loop (if any) so it recomputes its next
// deadline after a new schedule.
func (s *Scheduler) signalWake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// tick advances the underlying wheel to now and fires due tasks. It is a no-op
// once the scheduler is shut down.
func (s *Scheduler) tick(now time.Time) {
	if s.isShutdown() {
		return
	}
	s.wheel.Advance(now)
}
