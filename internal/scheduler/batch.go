package scheduler

import (
	"time"

	"timer-wheel/internal/wheel"
)

// BatchScheduler allows scheduling multiple timers at once.
type BatchScheduler struct {
	w       *wheel.Wheel
	pending []batchEntry
}

type batchEntry struct {
	id       int64
	delay    time.Duration
	priority int
	fn       wheel.TaskFunc
}

// NewBatch creates a batch scheduler wrapping the given wheel.
func NewBatch(w *wheel.Wheel) *BatchScheduler {
	return &BatchScheduler{w: w}
}

// Add queues a timer for batch submission.
func (bs *BatchScheduler) Add(id int64, delay time.Duration, priority int, fn wheel.TaskFunc) {
	bs.pending = append(bs.pending, batchEntry{id: id, delay: delay, priority: priority, fn: fn})
}

// Flush submits all pending timers to the wheel.
func (bs *BatchScheduler) Flush() int {
	count := len(bs.pending)
	for _, e := range bs.pending {
		bs.w.Add(e.id, e.delay, e.priority, e.fn)
	}
	bs.pending = bs.pending[:0]
	return count
}

// Pending returns the number of unflushed timers.
func (bs *BatchScheduler) Pending() int {
	return len(bs.pending)
}

// Clear discards all pending timers without scheduling them.
func (bs *BatchScheduler) Clear() {
	bs.pending = bs.pending[:0]
}
