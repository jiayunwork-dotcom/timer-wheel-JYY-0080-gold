package scheduler

import (
	"context"
	"time"

	"timer-wheel/internal/clock"
)

// Start begins driving the scheduler. With an advancing (fake) clock it hooks
// the clock's Advance so ticks are driven externally; with a plain real clock
// it spawns a goroutine that sleeps until the next deadline. Start is safe to
// call multiple times; only the first call takes effect.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.baseCtx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Align the wheel to the clock so delays are relative to "now".
	s.wheel.SetNow(s.clock.Now())

	if adv, ok := s.clock.(clock.AdvancingClock); ok {
		// Externally driven: every Advance of the fake clock ticks the wheel.
		adv.OnAdvance(func(now time.Time) { s.tick(now) })
		return
	}
	go s.loop(s.baseCtx)
}

// Run starts the scheduler and blocks until ctx is cancelled. It is the
// convenience entry point for a CLI or server main.
func (s *Scheduler) Run(ctx context.Context) {
	s.Start(ctx)
	<-ctx.Done()
}

// loop is the real-clock driver. It computes the next deadline from the wheel
// and waits (via the wall clock) for either that deadline, a new schedule, or
// context cancellation.
func (s *Scheduler) loop(ctx context.Context) {
	for {
		if s.isShutdown() {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		deadline := s.wheel.NextDeadline()
		if deadline.IsZero() {
			// No pending tasks: wait for a new schedule or shutdown.
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}

		delay := time.Until(deadline)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
			s.tick(s.clock.Now())
		}
	}
}
