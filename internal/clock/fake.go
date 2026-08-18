package clock

import (
	"sync"
	"time"
)

// fakeTimer is a single callback scheduled on a FakeClock.
type fakeTimer struct {
	id        int64
	fireAt    time.Time
	fn        func()
	cancelled bool
}

// FakeClock is a fully controllable Clock for tests and simulations. Time only
// moves when Advance is called, and every timer that becomes due is fired
// synchronously and in chronological order. Because nothing happens on the
// real wall clock, behaviour is 100% reproducible across runs.
//
// FakeClock is safe for concurrent use.
type FakeClock struct {
	mu        sync.Mutex
	now       time.Time
	seq       int64
	timers    []*fakeTimer
	listeners []func(time.Time)
}

// NewFakeClock builds a FakeClock anchored at t. All subsequent Add calls are
// relative to t.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

// Now implements Clock.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Add schedules fn to run delay after the current fake time. It returns a
// stable id that can be passed to Cancel. If delay is non-positive the timer
// is due immediately and will fire on the next Advance.
func (c *FakeClock) Add(delay time.Duration, fn func()) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	id := c.seq
	c.timers = append(c.timers, &fakeTimer{
		id:     id,
		fireAt: c.now.Add(delay),
		fn:     fn,
	})
	return id
}

// Cancel removes a previously scheduled timer before it fires. It reports
// whether a live timer with that id was actually cancelled. A timer that has
// already fired or was already cancelled cannot be cancelled again.
func (c *FakeClock) Cancel(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.timers {
		if t.id == id && !t.cancelled {
			t.cancelled = true
			return true
		}
	}
	return false
}

// Pending returns the number of timers that are still scheduled (not yet fired
// and not cancelled). Useful for deterministic test assertions.
func (c *FakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.cancelled {
			n++
		}
	}
	return n
}

// Advance moves the fake clock forward by d. Every timer whose fireAt is at or
// before the new time is invoked, in chronological order; if a callback
// schedules new timers, those are considered in the same Advance. Advance is a
// no-op (time does not move and listeners are not notified) when d is
// non-positive.
func (c *FakeClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	target := c.now.Add(d)

	for {
		idx := -1
		var earliest time.Time
		for i, t := range c.timers {
			if t.cancelled {
				continue
			}
			if idx == -1 || t.fireAt.Before(earliest) {
				idx = i
				earliest = t.fireAt
			}
		}
		if idx == -1 || earliest.After(target) {
			break
		}
		// Move the visible clock to the firing instant so Now() is correct
		// while callbacks execute.
		c.now = c.timers[idx].fireAt
		t := c.timers[idx]
		t.cancelled = true
		fn := t.fn
		c.timers = append(c.timers[:idx], c.timers[idx+1:]...)
		c.mu.Unlock()
		if fn != nil {
			fn()
		}
		c.mu.Lock()
	}
	c.now = target

	listeners := append([]func(time.Time){}, c.listeners...)
	c.mu.Unlock()
	for _, l := range listeners {
		l(target)
	}
}

// OnAdvance implements AdvancingClock by registering a listener invoked with
// the new current time after every Advance.
func (c *FakeClock) OnAdvance(fn func(time.Time)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, fn)
}

// Set jumps the clock to an absolute time without firing timers that lie
// between the old and new time. It exists for test setup convenience; most
// tests should prefer Advance. If t is before the current time, the clock is
// rewound.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

var _ AdvancingClock = (*FakeClock)(nil)
var _ Clock = RealClock{}
