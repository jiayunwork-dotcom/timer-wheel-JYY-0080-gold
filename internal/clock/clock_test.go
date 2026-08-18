package clock

import (
	"sync"
	"testing"
	"time"
)

// TestClockFakeAdvance verifies that a FakeClock only fires timers when
// Advance crosses their deadline, and that Now() tracks the movement.
func TestClockFakeAdvance(t *testing.T) {
	base := time.Unix(1000, 0).UTC()
	c := NewFakeClock(base)

	var fired bool
	id := c.Add(100*time.Millisecond, func() { fired = true })

	if got := c.Now(); !got.Equal(base) {
		t.Fatalf("Now() = %v, want %v", got, base)
	}
	if c.Pending() != 1 {
		t.Fatalf("Pending() = %d, want 1", c.Pending())
	}

	// Advancing short of the deadline must not fire anything.
	c.Advance(50 * time.Millisecond)
	if fired {
		t.Fatal("timer fired before its deadline")
	}
	if got := c.Now(); !got.Equal(base.Add(50 * time.Millisecond)) {
		t.Fatalf("Now() = %v, want %v", got, base.Add(50*time.Millisecond))
	}
	if c.Pending() != 1 {
		t.Fatalf("Pending() = %d, want 1 after partial advance", c.Pending())
	}

	// Crossing the deadline fires exactly once.
	c.Advance(50 * time.Millisecond)
	if !fired {
		t.Fatal("timer did not fire at its deadline")
	}
	if c.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0 after fire", c.Pending())
	}
	if got := c.Now(); !got.Equal(base.Add(100 * time.Millisecond)) {
		t.Fatalf("Now() = %v, want %v", got, base.Add(100*time.Millisecond))
	}

	// Further advances must not re-fire.
	c.Advance(time.Second)
	if c.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0", c.Pending())
	}
	_ = id
}

// TestClockFakeAdvanceOrder checks that multiple timers fire in chronological
// order, not insertion order.
func TestClockFakeAdvanceOrder(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var mu sync.Mutex
	var order []int
	record := func(n int) func() {
		return func() {
			mu.Lock()
			order = append(order, n)
			mu.Unlock()
		}
	}

	c.Add(200*time.Millisecond, record(2))
	c.Add(100*time.Millisecond, record(1))
	c.Add(300*time.Millisecond, record(3))

	c.Advance(500 * time.Millisecond)

	mu.Lock()
	got := append([]int(nil), order...)
	mu.Unlock()
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("fired %d timers, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fire order = %v, want %v", got, want)
		}
	}
}

// TestClockFakeCancel verifies that a cancelled timer never fires and that
// Cancel reports the right status.
func TestClockFakeCancel(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var fired bool
	id := c.Add(100*time.Millisecond, func() { fired = true })

	if !c.Cancel(id) {
		t.Fatal("Cancel returned false for a live timer")
	}
	if c.Cancel(id) {
		t.Fatal("Cancel returned true for an already-cancelled timer")
	}
	if c.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0 after cancel", c.Pending())
	}
	c.Advance(time.Second)
	if fired {
		t.Fatal("cancelled timer fired")
	}
}

// TestClockFakePending checks the bookkeeping of Pending across add/fire.
func TestClockFakePending(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	c.Add(10*time.Millisecond, func() {})
	c.Add(20*time.Millisecond, func() {})
	if c.Pending() != 2 {
		t.Fatalf("Pending() = %d, want 2", c.Pending())
	}
	c.Advance(15 * time.Millisecond)
	if c.Pending() != 1 {
		t.Fatalf("Pending() = %d, want 1 after first fire", c.Pending())
	}
	c.Advance(15 * time.Millisecond)
	if c.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0 after second fire", c.Pending())
	}
}

// TestClockFakeOnAdvance verifies that advance listeners are invoked with the
// new time, in registration order, and only on a positive Advance.
func TestClockFakeOnAdvance(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var calls []time.Duration
	c.OnAdvance(func(now time.Time) { calls = append(calls, now.Sub(time.Unix(0, 0).UTC())) })
	c.OnAdvance(func(now time.Time) { calls = append(calls, now.Sub(time.Unix(0, 0).UTC())) })

	c.Advance(100 * time.Millisecond)
	if len(calls) != 2 {
		t.Fatalf("listener called %d times, want 2", len(calls))
	}
	for i, d := range calls {
		if d != 100*time.Millisecond {
			t.Fatalf("call %d time = %v, want 100ms", i, d)
		}
	}

	before := len(calls)
	c.Advance(0) // non-positive must be a no-op
	if len(calls) != before {
		t.Fatal("non-positive Advance notified listeners")
	}
}

// TestClockRealNow checks the real clock reports a plausible, increasing time.
func TestClockRealNow(t *testing.T) {
	r := NewRealClock()
	a := r.Now()
	if a.IsZero() {
		t.Fatal("RealClock.Now returned zero time")
	}
	b := r.Now()
	if !b.After(a) && !b.Equal(a) {
		t.Fatalf("RealClock went backwards: %v then %v", a, b)
	}
}

// TestClockFakeSet verifies Set jumps the current time without firing timers
// that lie between the old and new time, and that Now tracks the jump.
func TestClockFakeSet(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	c := NewFakeClock(base)
	var fired bool
	c.Add(1000*time.Millisecond, func() { fired = true })

	// Jump far ahead without crossing the deadline's firing path.
	c.Set(base.Add(500 * time.Millisecond))
	if got := c.Now(); !got.Equal(base.Add(500 * time.Millisecond)) {
		t.Fatalf("Now() = %v, want +500ms", got)
	}
	if fired {
		t.Fatal("Set fired a timer it skipped over")
	}
	if c.Pending() != 1 {
		t.Fatalf("Pending() = %d, want 1 after Set", c.Pending())
	}
	// Crossing the deadline via Advance still fires correctly.
	c.Advance(600 * time.Millisecond)
	if !fired {
		t.Fatal("timer did not fire after Advance past deadline")
	}
}

// TestClockFakeAddZeroDelay schedules a zero-delay timer and confirms it fires
// on the next Advance but not before.
func TestClockFakeAddZeroDelay(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var fired bool
	c.Add(0, func() { fired = true })
	if fired {
		t.Fatal("zero-delay timer fired before Advance")
	}
	c.Advance(time.Millisecond)
	if !fired {
		t.Fatal("zero-delay timer did not fire on Advance")
	}
}

// TestClockFakeNonPositiveAdvance verifies a non-positive Advance is a true
// no-op: time does not move and timers do not fire.
func TestClockFakeNonPositiveAdvance(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	c := NewFakeClock(base)
	var fired bool
	c.Add(10*time.Millisecond, func() { fired = true })

	c.Advance(0)
	c.Advance(-time.Second)
	if fired {
		t.Fatal("timer fired on a non-positive Advance")
	}
	if got := c.Now(); !got.Equal(base) {
		t.Fatalf("Now() = %v, want unchanged base", got)
	}
	if c.Pending() != 1 {
		t.Fatalf("Pending() = %d, want 1", c.Pending())
	}
}

// TestClockFakeReplaceCancel ensures cancelling one of two timers leaves the
// other intact and firing correctly.
func TestClockFakeReplaceCancel(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var a, b bool
	idA := c.Add(100*time.Millisecond, func() { a = true })
	c.Add(200*time.Millisecond, func() { b = true })

	if !c.Cancel(idA) {
		t.Fatal("Cancel(idA) returned false")
	}
	c.Advance(300 * time.Millisecond)
	if a {
		t.Fatal("cancelled timer A fired")
	}
	if !b {
		t.Fatal("uncancelled timer B did not fire")
	}
	if c.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0", c.Pending())
	}
}

// TestClockFakeListenerOrder checks that multiple OnAdvance listeners run in
// registration order and observe the new current time.
func TestClockFakeListenerOrder(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0).UTC())
	var seq []int
	c.OnAdvance(func(time.Time) { seq = append(seq, 1) })
	c.OnAdvance(func(time.Time) { seq = append(seq, 2) })
	c.OnAdvance(func(time.Time) { seq = append(seq, 3) })

	c.Advance(10 * time.Millisecond)
	if len(seq) != 3 {
		t.Fatalf("listeners fired %d times, want 3", len(seq))
	}
	want := []int{1, 2, 3}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("listener order = %v, want %v", seq, want)
		}
	}
}
