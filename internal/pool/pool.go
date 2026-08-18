// Package pool implements a worker pool that executes timer callbacks. It
// decouples timer expiration detection (the wheel) from callback execution
// (the pool), allowing the wheel to remain lock-free while callbacks run
// concurrently in goroutine-like fashion (simulated here without real goroutines
// for determinism).
package pool

import (
	"errors"
	"sync"
	"sync/atomic"
)

// ErrClosed is returned when submitting to a closed pool.
var ErrClosed = errors.New("pool: closed")

// Task is a unit of work.
type Task struct {
	ID int64
	Fn func()
}

// Pool manages concurrent task execution.
type Pool struct {
	mu       sync.Mutex
	workers  int
	queue    []Task
	running  int32
	executed int64
	closed   bool
}

// New creates a pool with the given number of workers.
func New(workers int) *Pool {
	if workers <= 0 {
		workers = 4
	}
	return &Pool{workers: workers}
}

// Submit adds a task to the pool.
func (p *Pool) Submit(t Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	p.queue = append(p.queue, t)
	return nil
}

// Drain executes all queued tasks synchronously (for deterministic testing).
func (p *Pool) Drain() int {
	p.mu.Lock()
	tasks := make([]Task, len(p.queue))
	copy(tasks, p.queue)
	p.queue = p.queue[:0]
	p.mu.Unlock()

	for _, t := range tasks {
		atomic.AddInt32(&p.running, 1)
		t.Fn()
		atomic.AddInt32(&p.running, -1)
		atomic.AddInt64(&p.executed, 1)
	}
	return len(tasks)
}

// Pending returns the number of queued tasks.
func (p *Pool) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

// Running returns the number of currently executing tasks.
func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

// Executed returns the total number of completed tasks.
func (p *Pool) Executed() int64 {
	return atomic.LoadInt64(&p.executed)
}

// Close shuts down the pool. Pending tasks are discarded.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.queue = nil
}

// IsClosed reports whether the pool is closed.
func (p *Pool) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Workers returns the configured worker count.
func (p *Pool) Workers() int { return p.workers }

// Reset clears the queue and counters without closing.
func (p *Pool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = nil
	atomic.StoreInt64(&p.executed, 0)
}
