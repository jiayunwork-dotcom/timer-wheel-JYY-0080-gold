package scheduler

import (
	"sync"
	"sync/atomic"
	"time"

	"timer-wheel/internal/wheel"
)

// RepeatTimer is a self-rescheduling periodic timer.
type RepeatTimer struct {
	mu       sync.Mutex
	w        *wheel.Wheel
	id       int64
	interval time.Duration
	fn       wheel.TaskFunc
	stopped  int32
	count    int64
}

// NewRepeat creates a repeating timer. It fires fn every interval.
func NewRepeat(w *wheel.Wheel, id int64, interval time.Duration, fn wheel.TaskFunc) *RepeatTimer {
	rt := &RepeatTimer{
		w:        w,
		id:       id,
		interval: interval,
		fn:       fn,
	}
	rt.schedule()
	return rt
}

// Stop cancels the repeating timer.
func (rt *RepeatTimer) Stop() {
	atomic.StoreInt32(&rt.stopped, 1)
	rt.w.Cancel(rt.id)
}

// IsStopped reports whether the timer has been stopped.
func (rt *RepeatTimer) IsStopped() bool {
	return atomic.LoadInt32(&rt.stopped) == 1
}

// Count returns how many times the timer has fired.
func (rt *RepeatTimer) Count() int64 {
	return atomic.LoadInt64(&rt.count)
}

// Interval returns the repeat interval.
func (rt *RepeatTimer) Interval() time.Duration {
	return rt.interval
}

func (rt *RepeatTimer) schedule() {
	rt.w.Add(rt.id, rt.interval, 0, rt.fire)
}

func (rt *RepeatTimer) fire() {
	if rt.IsStopped() {
		return
	}
	atomic.AddInt64(&rt.count, 1)
	rt.fn()
	if !rt.IsStopped() {
		rt.schedule()
	}
}
