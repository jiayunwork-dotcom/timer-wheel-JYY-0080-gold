package metrics

import "time"

// Histogram tracks the distribution of timer delays.
type Histogram struct {
	buckets []int64
	bounds  []time.Duration
}

// NewHistogram creates a histogram with the given bucket boundaries.
func NewHistogram(bounds []time.Duration) *Histogram {
	return &Histogram{
		buckets: make([]int64, len(bounds)+1),
		bounds:  bounds,
	}
}

// DefaultHistogram creates a histogram with common boundaries.
func DefaultHistogram() *Histogram {
	return NewHistogram([]time.Duration{
		time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		time.Second,
		10 * time.Second,
		time.Minute,
		10 * time.Minute,
		time.Hour,
	})
}

// Observe records a duration.
func (h *Histogram) Observe(d time.Duration) {
	idx := len(h.bounds) // overflow bucket
	for i, b := range h.bounds {
		if d <= b {
			idx = i
			break
		}
	}
	h.buckets[idx]++
}

// Counts returns the bucket counts.
func (h *Histogram) Counts() []int64 {
	out := make([]int64, len(h.buckets))
	copy(out, h.buckets)
	return out
}

// Total returns the sum of all observations.
func (h *Histogram) Total() int64 {
	var t int64
	for _, c := range h.buckets {
		t += c
	}
	return t
}

// Reset zeroes all buckets.
func (h *Histogram) Reset() {
	for i := range h.buckets {
		h.buckets[i] = 0
	}
}

// BucketCount returns the number of buckets.
func (h *Histogram) BucketCount() int {
	return len(h.buckets)
}
