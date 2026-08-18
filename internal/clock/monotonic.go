package clock

import "time"

// Monotonic is a clock that only moves forward, never backwards.
// It wraps another clock and filters out any backward jumps.
type Monotonic struct {
	inner Clock
	last  time.Time
}

// NewMonotonic wraps a clock to make it monotonic.
func NewMonotonic(c Clock) *Monotonic {
	return &Monotonic{inner: c, last: c.Now()}
}

// Now returns the current time, guaranteed >= last returned time.
func (m *Monotonic) Now() time.Time {
	t := m.inner.Now()
	if t.Before(m.last) {
		return m.last
	}
	m.last = t
	return t
}

// Since returns the duration since t, using the monotonic clock.
func (m *Monotonic) Since(t time.Time) time.Duration {
	return m.Now().Sub(t)
}

// Elapsed returns time elapsed since the clock was created.
func (m *Monotonic) Elapsed() time.Duration {
	return m.Now().Sub(m.last)
}

// Ticker creates a channel that receives ticks at the given interval.
// (Simulated: returns values on demand for testing.)
type Ticker struct {
	interval time.Duration
	clock    Clock
	last     time.Time
}

// NewTicker creates a ticker.
func NewTicker(c Clock, interval time.Duration) *Ticker {
	return &Ticker{interval: interval, clock: c, last: c.Now()}
}

// Ready reports whether a tick is due.
func (t *Ticker) Ready() bool {
	now := t.clock.Now()
	return now.Sub(t.last) >= t.interval
}

// Tick resets the ticker and returns the current time.
func (t *Ticker) Tick() time.Time {
	now := t.clock.Now()
	t.last = now
	return now
}

// Interval returns the tick interval.
func (t *Ticker) Interval() time.Duration {
	return t.interval
}
