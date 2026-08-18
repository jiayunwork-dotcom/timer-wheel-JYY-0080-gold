package wheel

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func newTestWheel() *Wheel {
	return New(Options{Tick: time.Millisecond, WheelSize: 20, Start: time.Unix(0, 0).UTC()})
}

// TestWheelAddTick verifies a task fires exactly when its delay elapses and not
// before.
func TestWheelAddTick(t *testing.T) {
	w := newTestWheel()
	var fired bool
	w.Add(1, 50*time.Millisecond, 0, func() { fired = true })

	if w.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", w.Len())
	}
	w.AdvanceBy(49 * time.Millisecond)
	if fired {
		t.Fatal("task fired before its delay elapsed")
	}
	w.AdvanceBy(1 * time.Millisecond)
	if !fired {
		t.Fatal("task did not fire at its delay")
	}
	if w.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after fire", w.Len())
	}
}

// TestWheelCancel verifies a cancelled task never fires and frees its slot.
func TestWheelCancel(t *testing.T) {
	w := newTestWheel()
	var fired bool
	w.Add(1, 100*time.Millisecond, 0, func() { fired = true })

	if !w.Cancel(1) {
		t.Fatal("Cancel returned false for a live task")
	}
	if w.Cancel(1) {
		t.Fatal("Cancel returned true for an already-cancelled task")
	}
	if w.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after cancel", w.Len())
	}
	w.AdvanceBy(time.Second)
	if fired {
		t.Fatal("cancelled task fired")
	}
}

// TestWheelExpireOrder schedules tasks at varied delays spanning multiple
// levels/buckets and asserts they fire in chronological (deadline) order.
func TestWheelExpireOrder(t *testing.T) {
	w := newTestWheel()
	type spec struct {
		id   int64
		delay time.Duration
	}
	specs := []spec{
		{1, 10 * time.Millisecond},
		{2, 100 * time.Millisecond},
		{3, 50 * time.Millisecond},
		{4, 1000 * time.Millisecond},
		{5, 2000 * time.Millisecond},
		{6, 30 * time.Millisecond},
		{7, 500 * time.Millisecond},
		{8, 200 * time.Millisecond},
		{9, 1500 * time.Millisecond},
	}
	var mu sync.Mutex
	var order []int64
	for _, s := range specs {
		s := s
		w.Add(s.id, s.delay, 0, func() {
			mu.Lock()
			order = append(order, s.id)
			mu.Unlock()
		})
	}

	// Advance in one large jump so every task is due.
	w.AdvanceBy(3000 * time.Millisecond)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()

	if len(got) != len(specs) {
		t.Fatalf("fired %d tasks, want %d (%v)", len(got), len(specs), got)
	}
	// Expected order is by ascending delay.
	want := []int64{1, 6, 3, 2, 8, 7, 4, 9, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fire order = %v, want %v", got, want)
		}
	}
}

// TestWheelMultipleBuckets ensures tasks landing in distinct buckets of the
// same level still fire in deadline order.
func TestWheelMultipleBuckets(t *testing.T) {
	w := newTestWheel()
	var mu sync.Mutex
	var order []int64
	for i := int64(1); i <= 20; i++ {
		i := i
		// Spread delays so each lands in a different bucket (i ms each).
		w.Add(i, time.Duration(i)*time.Millisecond, 0, func() {
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
		})
	}
	w.AdvanceBy(100 * time.Millisecond)
	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	if len(got) != 20 {
		t.Fatalf("fired %d tasks, want 20", len(got))
	}
	for i := range got {
		if got[i] != int64(i+1) {
			t.Fatalf("fire order = %v, want sequential 1..20", got)
		}
	}
}

// TestWheelCancelOverflow cancels a task that was placed in a higher (overflow)
// level and verifies it never fires.
func TestWheelCancelOverflow(t *testing.T) {
	w := newTestWheel()
	var fired bool
	// 5000ms overflows past level 0 (20ms) and level 1 (400ms) into level 2.
	w.Add(1, 5000*time.Millisecond, 0, func() { fired = true })

	if !w.Cancel(1) {
		t.Fatal("Cancel returned false for an overflow-level task")
	}
	w.AdvanceBy(10 * time.Second)
	if fired {
		t.Fatal("cancelled overflow task fired")
	}
}

// TestWheelReplace verifies that re-adding the same id replaces the prior task.
func TestWheelReplace(t *testing.T) {
	w := newTestWheel()
	var first, second bool
	w.Add(1, 50*time.Millisecond, 0, func() { first = true })
	w.Add(1, 10*time.Millisecond, 0, func() { second = true })

	if w.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 after replace", w.Len())
	}
	w.AdvanceBy(100 * time.Millisecond)
	if first {
		t.Fatal("old task (replaced) fired")
	}
	if !second {
		t.Fatal("replacement task did not fire")
	}
}

// TestWheelNegativeDelay treats a negative delay as immediate and fires on the
// next Advance.
func TestWheelNegativeDelay(t *testing.T) {
	w := newTestWheel()
	var fired bool
	w.Add(1, -time.Second, 0, func() { fired = true })
	if !fired {
		// Not fired until Advance.
		w.AdvanceBy(time.Millisecond)
	}
	if !fired {
		t.Fatal("non-positive delay task did not fire on Advance")
	}
}

// TestWheelPendingIDs checks the snapshot of scheduled ids.
func TestWheelPendingIDs(t *testing.T) {
	w := newTestWheel()
	w.Add(11, 10*time.Millisecond, 0, func() {})
	w.Add(22, 20*time.Millisecond, 0, func() {})
	ids := w.PendingIDs()
	if len(ids) != 2 {
		t.Fatalf("PendingIDs() = %d, want 2", len(ids))
	}
	w.Cancel(11)
	if got := w.PendingIDs(); len(got) != 1 {
		t.Fatalf("PendingIDs() after cancel = %d, want 1", len(got))
	}
}

// TestWheelPriorityOrder schedules several tasks that share the exact same
// deadline but differ in priority, and asserts they fire in descending priority
// order (ties broken by ascending id).
func TestWheelPriorityOrder(t *testing.T) {
	w := newTestWheel()
	var mu sync.Mutex
	var order []int64
	record := func(id int64) func() {
		return func() {
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
		}
	}

	// All due at the same instant; priorities ascend with id by default but we
	// invert them to prove priority wins over insertion order.
	w.Add(3, 100*time.Millisecond, 1, record(3))
	w.Add(1, 100*time.Millisecond, 9, record(1))
	w.Add(2, 100*time.Millisecond, 5, record(2))

	w.AdvanceBy(200 * time.Millisecond)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	want := []int64{1, 2, 3} // priority 9, then 5, then 1
	if len(got) != len(want) {
		t.Fatalf("fired %d tasks, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("priority fire order = %v, want %v", got, want)
		}
	}
}

// TestWheelPriorityEqualTieBreak schedules two equal-deadline, equal-priority
// tasks and asserts they fire in ascending id order (the deterministic tiebreak).
func TestWheelPriorityEqualTieBreak(t *testing.T) {
	w := newTestWheel()
	var mu sync.Mutex
	var order []int64
	w.Add(20, 100*time.Millisecond, 0, func() { mu.Lock(); order = append(order, 20); mu.Unlock() })
	w.Add(7, 100*time.Millisecond, 0, func() { mu.Lock(); order = append(order, 7); mu.Unlock() })

	w.AdvanceBy(200 * time.Millisecond)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	want := []int64{7, 20}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tiebreak fire order = %v, want %v", got, want)
	}
}

// TestWheelAdvancePastZero ensures scheduling at delay 0 fires on the very next
// Advance regardless of how far back the clock sits.
func TestWheelAdvancePastZero(t *testing.T) {
	w := newTestWheel()
	var fired bool
	w.Add(1, 0, 0, func() { fired = true })
	w.AdvanceBy(5 * time.Millisecond)
	if !fired {
		t.Fatal("zero-delay task did not fire on Advance")
	}
	if w.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after fire", w.Len())
	}
}

// TestWheelNowSetNow checks the clock anchor can be moved before scheduling, and
// that delays are interpreted relative to the new anchor.
func TestWheelNowSetNow(t *testing.T) {
	w := newTestWheel()
	anchor := time.Unix(5000, 0).UTC()
	w.SetNow(anchor)
	if got := w.Now(); !got.Equal(anchor) {
		t.Fatalf("Now() = %v, want %v", got, anchor)
	}
	var fired bool
	// Deadline is anchor + 1s = Unix(5001, 0).
	w.Add(1, time.Second, 0, func() { fired = true })
	// Advancing to 500ms past the anchor is still short of the deadline.
	w.Advance(anchor.Add(500 * time.Millisecond))
	if fired {
		t.Fatal("task fired before its absolute deadline")
	}
	w.Advance(anchor.Add(1500 * time.Millisecond))
	if !fired {
		t.Fatal("task did not fire at its absolute deadline")
	}
}
