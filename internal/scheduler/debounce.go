package scheduler

import (
	"sync"
	"time"

	"timer-wheel/internal/wheel"
)

// Debouncer schedules a callback that only fires after a quiet period. Each
// call to Trigger resets the delay. This is useful for rate-limiting events
// like config reloads or save operations.
type Debouncer struct {
	mu       sync.Mutex
	w        *wheel.Wheel
	id       int64
	delay    time.Duration
	fn       wheel.TaskFunc
	active   bool
}

// NewDebouncer creates a debouncer.
func NewDebouncer(w *wheel.Wheel, id int64, delay time.Duration, fn wheel.TaskFunc) *Debouncer {
	return &Debouncer{w: w, id: id, delay: delay, fn: fn}
}

// Trigger starts or restarts the debounce timer.
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active {
		d.w.Cancel(d.id)
	}
	d.w.Add(d.id, d.delay, 0, d.fn)
	d.active = true
}

// Cancel cancels the pending debounce without firing.
func (d *Debouncer) Cancel() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active {
		d.w.Cancel(d.id)
		d.active = false
	}
}

// IsActive reports whether a debounce is pending.
func (d *Debouncer) IsActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

// Delay returns the configured debounce delay.
func (d *Debouncer) Delay() time.Duration {
	return d.delay
}
