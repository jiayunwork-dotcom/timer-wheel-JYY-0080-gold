// Package clock provides an abstraction over time so that schedulers and
// timing wheels can be driven by either the real wall clock or a fully
// controllable fake clock. Determinism is the central design goal: every
// component in this project that depends on time receives a Clock, never the
// time package directly. That makes the whole scheduling stack testable
// without sleeping on the real wall clock.
package clock

import "time"

// Clock is the minimal time abstraction used across this project. Any type
// that can report "the current time" satisfies it.
type Clock interface {
	// Now returns the current time as seen by this clock.
	Now() time.Time
}

// RealClock reports the actual system wall clock. It is safe for concurrent
// use and has no internal state.
type RealClock struct{}

// NewRealClock constructs a RealClock.
func NewRealClock() RealClock { return RealClock{} }

// Now implements Clock by returning time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// AdvancingClock is a Clock that can also be moved forward explicitly. Fake
// clocks satisfy it; the real clock does not. Callers that want to drive time
// deterministically (tests, simulations) can type-assert a Clock to this
// interface and use Advance / OnAdvance.
type AdvancingClock interface {
	Clock

	// Advance moves the clock forward by d and fires anything that becomes due
	// as a result, in deterministic chronological order. It is a no-op (other
	// than clamping) if d is non-positive.
	Advance(d time.Duration)

	// OnAdvance registers a callback invoked after every Advance with the new
	// current time. Listeners are notified in registration order. This is how
	// higher level schedulers hook into the fake clock: when the test advances
	// time, the scheduler ticks its wheel.
	OnAdvance(fn func(time.Time))
}

// zero is a sentinel used when a time value must be "unset".
var epoch = time.Unix(0, 0).UTC()
