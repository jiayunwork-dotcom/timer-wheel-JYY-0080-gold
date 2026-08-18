// Package metrics collects runtime statistics for the timer wheel: fired count,
// cancelled count, latency distribution, and bucket utilization.
package metrics

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// Counters holds atomic operation counters.
type Counters struct {
	Added     atomic.Int64
	Fired     atomic.Int64
	Cancelled atomic.Int64
	Expired   atomic.Int64
	Missed    atomic.Int64
}

// Snapshot is a point-in-time copy.
type Snapshot struct {
	Added     int64     `json:"added"`
	Fired     int64     `json:"fired"`
	Cancelled int64     `json:"cancelled"`
	Expired   int64     `json:"expired"`
	Missed    int64     `json:"missed"`
	TakenAt   time.Time `json:"taken_at"`
}

// Take returns current counter values.
func (c *Counters) Take() Snapshot {
	return Snapshot{
		Added:     c.Added.Load(),
		Fired:     c.Fired.Load(),
		Cancelled: c.Cancelled.Load(),
		Expired:   c.Expired.Load(),
		Missed:    c.Missed.Load(),
		TakenAt:   time.Now(),
	}
}

// Reset zeroes all counters.
func (c *Counters) Reset() {
	c.Added.Store(0)
	c.Fired.Store(0)
	c.Cancelled.Store(0)
	c.Expired.Store(0)
	c.Missed.Store(0)
}

// JSON returns the counters as formatted JSON.
func (c *Counters) JSON() ([]byte, error) {
	return json.MarshalIndent(c.Take(), "", "  ")
}

// LatencyTracker records timer latencies (delay between scheduled and actual fire).
type LatencyTracker struct {
	mu      sync.Mutex
	samples []time.Duration
	maxLen  int
}

// NewLatencyTracker creates a tracker keeping the last maxLen samples.
func NewLatencyTracker(maxLen int) *LatencyTracker {
	if maxLen <= 0 {
		maxLen = 1000
	}
	return &LatencyTracker{samples: make([]time.Duration, 0, maxLen), maxLen: maxLen}
}

// Record adds a latency sample.
func (lt *LatencyTracker) Record(d time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if len(lt.samples) >= lt.maxLen {
		copy(lt.samples, lt.samples[1:])
		lt.samples = lt.samples[:len(lt.samples)-1]
	}
	lt.samples = append(lt.samples, d)
}

// Count returns recorded samples.
func (lt *LatencyTracker) Count() int {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return len(lt.samples)
}

// Avg returns the average latency.
func (lt *LatencyTracker) Avg() time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if len(lt.samples) == 0 {
		return 0
	}
	var total time.Duration
	for _, s := range lt.samples {
		total += s
	}
	return total / time.Duration(len(lt.samples))
}

// Max returns the maximum latency.
func (lt *LatencyTracker) Max() time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	var max time.Duration
	for _, s := range lt.samples {
		if s > max {
			max = s
		}
	}
	return max
}

// P99 returns an approximate 99th percentile (simple: largest 1%).
func (lt *LatencyTracker) P99() time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if len(lt.samples) == 0 {
		return 0
	}
	// Simple: sort a copy and pick the 99th percentile index.
	sorted := make([]time.Duration, len(lt.samples))
	copy(sorted, lt.samples)
	// Insertion sort for simplicity.
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := len(sorted)*99/100 - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// Reset clears all samples.
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.samples = lt.samples[:0]
}
