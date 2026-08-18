// Command example demonstrates the timer-wheel library with an offline,
// fully deterministic scenario driven by a fake clock. Because the clock is
// injectable, the exact firing order is reproducible and no real time is
// consumed.
//
// Run it with:
//
//	go run ./example
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"timer-wheel/internal/clock"
	"timer-wheel/internal/scheduler"
)

// demoFakeClock shows how a fake clock makes scheduling deterministic and
// testable. Tasks are scheduled at assorted delays; advancing the clock fires
// them in chronological order.
func demoFakeClock() {
	fmt.Println("== deterministic demo (fake clock) ==")
	fc := clock.NewFakeClock(time.Unix(1_000_000, 0).UTC())
	s := scheduler.New(fc, scheduler.Options{})
	s.Start(context.Background())

	type job struct {
		id    int64
		delay time.Duration
	}
	jobs := []job{
		{1, 300 * time.Millisecond},
		{2, 50 * time.Millisecond},
		{3, 120 * time.Millisecond},
		{4, 5000 * time.Millisecond}, // overflows the lower wheel levels
		{5, 10 * time.Millisecond},
	}

	var mu sync.Mutex
	var fired []int64
	for _, j := range jobs {
		j := j
		if _, err := s.Schedule(j.delay, func() {
			mu.Lock()
			fired = append(fired, j.id)
			mu.Unlock()
		}); err != nil {
			panic(err)
		}
	}

	// Cancel job 3 before it is due.
	if !s.Cancel(3) {
		panic("expected job 3 to be cancellable")
	}

	// Advance the clock well past every delay.
	fc.Advance(10 * time.Second)

	mu.Lock()
	ordered := append([]int64(nil), fired...)
	mu.Unlock()

	fmt.Printf("fired (ids in fire order): %v\n", ordered)
	fmt.Printf("metrics:                  %+v\n", s.Metrics())
	fmt.Println("note: job 3 was cancelled, so only 5,2,1,4 should have fired, in delay order")
}

// demoChained shows that a task may schedule further work from within its own
// callback; the new task is due relative to the (fake) clock at fire time.
func demoChained() {
	fmt.Println("\n== chained scheduling demo (fake clock) ==")
	fc := clock.NewFakeClock(time.Unix(0, 0).UTC())
	s := scheduler.New(fc, scheduler.Options{})
	s.Start(context.Background())

	var mu sync.Mutex
	var events []string
	s.Schedule(100*time.Millisecond, func() {
		mu.Lock()
		events = append(events, "first")
		mu.Unlock()
		// Schedule a follow-up from inside the callback.
		_, _ = s.Schedule(100*time.Millisecond, func() {
			mu.Lock()
			events = append(events, "second")
			mu.Unlock()
		})
	})

	fc.Advance(50 * time.Millisecond)
	fc.Advance(100 * time.Millisecond) // fires "first" and schedules "second"
	fc.Advance(100 * time.Millisecond) // fires "second"

	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	fmt.Printf("events: %v\n", got)
}

func main() {
	demoFakeClock()
	demoChained()
}
