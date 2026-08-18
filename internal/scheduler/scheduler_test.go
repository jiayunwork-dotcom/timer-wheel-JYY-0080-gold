package scheduler

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"timer-wheel/internal/clock"
)

// TestSchedulerSchedule schedules a single task and confirms it fires when the
// (fake) clock advances past its delay, with metrics updated.
func TestSchedulerSchedule(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(1000, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var fired bool
	id, err := s.Schedule(100*time.Millisecond, func() { fired = true })
	if err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	if id != 1 {
		t.Fatalf("first id = %d, want 1", id)
	}
	if m := s.Metrics(); m.Scheduled != 1 {
		t.Fatalf("Scheduled = %d, want 1", m.Scheduled)
	}
	if fired {
		t.Fatal("task fired before advance")
	}

	fc.Advance(100 * time.Millisecond)
	if !fired {
		t.Fatal("task did not fire after advance")
	}
	if m := s.Metrics(); m.Fired != 1 {
		t.Fatalf("Fired = %d, want 1", m.Fired)
	}
}

// TestSchedulerCancel schedules two tasks, cancels one before it is due, and
// confirms only the surviving task fires.
func TestSchedulerCancel(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var a, b bool
	if _, err := s.Schedule(100*time.Millisecond, func() { a = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Schedule(50*time.Millisecond, func() { b = true }); err != nil {
		t.Fatal(err)
	}

	if !s.Cancel(1) {
		t.Fatal("Cancel(1) returned false for a live task")
	}
	if s.Cancel(1) {
		t.Fatal("Cancel(1) returned true for an already-cancelled task")
	}
	if s.Cancel(999) {
		t.Fatal("Cancel(999) returned true for an unknown id")
	}

	fc.Advance(200 * time.Millisecond)
	if a {
		t.Fatal("cancelled task (id 1) fired")
	}
	if !b {
		t.Fatal("uncancelled task (id 2) did not fire")
	}
	if m := s.Metrics(); m.Cancelled != 1 || m.Fired != 1 {
		t.Fatalf("metrics = %+v, want Cancelled=1 Fired=1", m)
	}
}

// TestSchedulerShutdown cancels all pending tasks on Shutdown, propagates
// context cancellation, and fires none of them.
func TestSchedulerShutdown(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var fired int
	for i := 0; i < 5; i++ {
		if _, err := s.Schedule(time.Duration(i+1)*time.Second, func() { fired++ }); err != nil {
			t.Fatal(err)
		}
	}
	_ = fired

	s.Shutdown()

	fc.Advance(10 * time.Second)
	if fired != 0 {
		t.Fatalf("fired %d tasks after Shutdown, want 0", fired)
	}
	m := s.Metrics()
	if m.Scheduled != 5 {
		t.Fatalf("Scheduled = %d, want 5", m.Scheduled)
	}
	if m.Fired != 0 {
		t.Fatalf("Fired = %d, want 0", m.Fired)
	}
	if m.Cancelled != 5 {
		t.Fatalf("Cancelled = %d, want 5", m.Cancelled)
	}

	select {
	case <-s.Done():
		// expected: context was cancelled by Shutdown
	case <-time.After(10 * time.Millisecond):
		t.Fatal("scheduler context was not cancelled by Shutdown")
	}
}

// TestSchedulerCancelPropagation cancels a task before its deadline and asserts
// it is never fired while sibling tasks still fire in order.
func TestSchedulerCancelPropagation(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var mu sync.Mutex
	var order []int64
	schedule := func(id int64, d time.Duration) {
		_, _ = s.Schedule(d, func() {
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
		})
	}
	schedule(1, 10*time.Millisecond)
	schedule(2, 20*time.Millisecond)
	schedule(3, 30*time.Millisecond)

	// Cancel the middle task before it becomes due.
	if !s.Cancel(2) {
		t.Fatal("Cancel(2) failed")
	}

	fc.Advance(100 * time.Millisecond)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	want := []int64{1, 3}
	if len(got) != len(want) {
		t.Fatalf("fired ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fired ids = %v, want %v", got, want)
		}
	}
	if m := s.Metrics(); m.Cancelled != 1 {
		t.Fatalf("Cancelled = %d, want 1", m.Cancelled)
	}
}

// TestSchedulerBadInput verifies the library rejects malformed input instead of
// misfiring.
func TestSchedulerBadInput(t *testing.T) {
	s := New(clock.NewFakeClock(time.Unix(0, 0).UTC()), Options{})
	if _, err := s.Schedule(-time.Second, func() {}); err == nil {
		t.Fatal("negative delay was accepted")
	}
	if _, err := s.Schedule(time.Second, nil); err == nil {
		t.Fatal("nil callback was accepted")
	}
}

// TestSchedulerMetrics exercises schedule/cancel/fire combinations and checks
// the cumulative counters stay consistent.
func TestSchedulerMetrics(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var fired int
	// 3 fire, 2 cancelled.
	s.Schedule(10*time.Millisecond, func() { fired++ })
	s.Schedule(20*time.Millisecond, func() { fired++ })
	s.Schedule(30*time.Millisecond, func() { fired++ })
	id4, _ := s.Schedule(40*time.Millisecond, func() { fired++ })
	id5, _ := s.Schedule(50*time.Millisecond, func() { fired++ })

	s.Cancel(id4)
	s.Cancel(id5)

	fc.Advance(100 * time.Millisecond)
	if fired != 3 {
		t.Fatalf("fired %d, want 3", fired)
	}
	m := s.Metrics()
	if m.Scheduled != 5 || m.Fired != 3 || m.Cancelled != 2 {
		t.Fatalf("metrics = %+v, want Scheduled=5 Fired=3 Cancelled=2", m)
	}
}

// TestSchedulerMultiLevel drives tasks with delays large enough to overflow the
// lower wheel levels and verifies they still fire in chronological order.
func TestSchedulerMultiLevel(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{Tick: time.Millisecond, WheelSize: 20})
	s.Start(context.Background())

	var mu sync.Mutex
	var order []int64
	type spec struct {
		id   int64
		delay time.Duration
	}
	specs := []spec{
		{1, 500 * time.Millisecond},
		{2, 5 * time.Second},
		{3, 2 * time.Second},
		{4, 1500 * time.Millisecond},
	}
	for _, sp := range specs {
		sp := sp
		_, _ = s.Schedule(sp.delay, func() {
			mu.Lock()
			order = append(order, sp.id)
			mu.Unlock()
		})
	}

	fc.Advance(10 * time.Second)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	want := []int64{1, 4, 3, 2}
	if len(got) != len(want) {
		t.Fatalf("fired %d tasks, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fire order = %v, want %v", got, want)
		}
	}
}

// TestSchedulerPriorityOrder schedules tasks sharing the same delay but with
// different priorities and asserts they fire in descending priority order.
func TestSchedulerPriorityOrder(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var mu sync.Mutex
	var order []int64
	record := func(id int64) func() {
		return func() {
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
		}
	}

	// id 3 lowest priority, id 1 highest; all due at +100ms.
	if _, err := s.ScheduleWith(100*time.Millisecond, ScheduleOptions{Priority: 1}, record(3)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScheduleWith(100*time.Millisecond, ScheduleOptions{Priority: 9}, record(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScheduleWith(100*time.Millisecond, ScheduleOptions{Priority: 5}, record(2)); err != nil {
		t.Fatal(err)
	}

	fc.Advance(200 * time.Millisecond)

	mu.Lock()
	got := append([]int64(nil), order...)
	mu.Unlock()
	want := []int64{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("priority fire order = %v, want %v", got, want)
	}
}

// TestSchedulerStats exercises the live-state snapshot: queue depth, next
// deadline, and the running flag, alongside the cumulative counters.
func TestSchedulerStats(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{Tick: time.Millisecond, WheelSize: 20})
	s.Start(context.Background())

	st := s.Stats()
	if !st.Running {
		t.Fatal("Stats.Running = false after Start")
	}
	if st.Pending != 0 {
		t.Fatalf("Pending = %d, want 0", st.Pending)
	}

	idA, _ := s.Schedule(100*time.Millisecond, func() {})
	_, _ = s.Schedule(50*time.Millisecond, func() {})

	st = s.Stats()
	if st.Pending != 2 {
		t.Fatalf("Pending = %d, want 2", st.Pending)
	}
	if st.Scheduled != 2 {
		t.Fatalf("Scheduled = %d, want 2", st.Scheduled)
	}
	wantDeadline := fc.Now().Add(50 * time.Millisecond)
	if !st.NextDeadline.Equal(wantDeadline) {
		t.Fatalf("NextDeadline = %v, want %v", st.NextDeadline, wantDeadline)
	}

	// Cancel one; pending drops, cancelled counter rises.
	if !s.Cancel(idA) {
		t.Fatal("Cancel(idA) failed")
	}
	st = s.Stats()
	if st.Pending != 1 {
		t.Fatalf("Pending after cancel = %d, want 1", st.Pending)
	}
	if st.Cancelled != 1 {
		t.Fatalf("Cancelled = %d, want 1", st.Cancelled)
	}

	// Fire the rest.
	fc.Advance(200 * time.Millisecond)
	st = s.Stats()
	if st.Pending != 0 {
		t.Fatalf("Pending after fire = %d, want 0", st.Pending)
	}
	if st.Fired != 1 {
		t.Fatalf("Fired = %d, want 1", st.Fired)
	}
}

// TestSchedulerStatsNextDeadlineEmpty verifies NextDeadline is the zero time when
// no tasks are pending.
func TestSchedulerStatsNextDeadlineEmpty(t *testing.T) {
	s := New(clock.NewFakeClock(time.Unix(0, 0).UTC()), Options{})
	s.Start(context.Background())
	st := s.Stats()
	if !st.NextDeadline.IsZero() {
		t.Fatalf("NextDeadline = %v, want zero time", st.NextDeadline)
	}
	if st.Pending != 0 {
		t.Fatalf("Pending = %d, want 0", st.Pending)
	}
}

// TestSchedulerScheduleWithLabel confirms ScheduleWith accepts a label and that
// the call still schedules and fires normally.
func TestSchedulerScheduleWithLabel(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := New(fc, Options{})
	s.Start(context.Background())

	var fired bool
	id, err := s.ScheduleWith(50*time.Millisecond, ScheduleOptions{Label: "heartbeat"}, func() { fired = true })
	if err != nil {
		t.Fatalf("ScheduleWith returned error: %v", err)
	}
	if id != 1 {
		t.Fatalf("first id = %d, want 1", id)
	}
	fc.Advance(100 * time.Millisecond)
	if !fired {
		t.Fatal("labelled task did not fire")
	}
}

// TestSchedulerScheduleWithNegativeDelay ensures the priority-aware entry point
// still rejects negative delays.
func TestSchedulerScheduleWithNegativeDelay(t *testing.T) {
	s := New(clock.NewFakeClock(time.Unix(0, 0).UTC()), Options{})
	if _, err := s.ScheduleWith(-time.Second, ScheduleOptions{Priority: 7}, func() {}); err == nil {
		t.Fatal("negative delay with priority was accepted")
	}
}
