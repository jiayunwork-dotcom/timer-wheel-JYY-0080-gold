// Package heap implements a min-heap of timers ordered by deadline. It serves
// as an alternative to the hierarchical wheel for very long-delay timers that
// would otherwise cascade through many wheel levels.
package heap

import (
	"container/heap"
	"sync"
	"time"
)

// Timer is a scheduled timer entry.
type Timer struct {
	ID       int64
	Deadline time.Time
	Fn       func()
	index    int
}

// MinHeap is a thread-safe priority queue of timers.
type MinHeap struct {
	mu    sync.Mutex
	items timerHeap
}

// New creates an empty timer heap.
func New() *MinHeap {
	h := &MinHeap{}
	heap.Init(&h.items)
	return h
}

// Push adds a timer.
func (h *MinHeap) Push(t *Timer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	heap.Push(&h.items, t)
}

// Pop removes and returns the timer with the earliest deadline.
func (h *MinHeap) Pop() *Timer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.items.Len() == 0 {
		return nil
	}
	return heap.Pop(&h.items).(*Timer)
}

// Peek returns the earliest timer without removing it.
func (h *MinHeap) Peek() *Timer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.items.Len() == 0 {
		return nil
	}
	return h.items[0]
}

// Len returns the number of timers.
func (h *MinHeap) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.items.Len()
}

// Remove removes a timer by ID. Returns true if found.
func (h *MinHeap) Remove(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, t := range h.items {
		if t.ID == id {
			heap.Remove(&h.items, i)
			return true
		}
	}
	return false
}

// DrainExpired removes and returns all timers whose deadline <= now.
func (h *MinHeap) DrainExpired(now time.Time) []*Timer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var expired []*Timer
	for h.items.Len() > 0 && !h.items[0].Deadline.After(now) {
		expired = append(expired, heap.Pop(&h.items).(*Timer))
	}
	return expired
}

// Clear removes all timers.
func (h *MinHeap) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = nil
}

// --- heap.Interface implementation ---

type timerHeap []*Timer

func (h timerHeap) Len() int            { return len(h) }
func (h timerHeap) Less(i, j int) bool   { return h[i].Deadline.Before(h[j].Deadline) }
func (h timerHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *timerHeap) Push(x interface{})  { t := x.(*Timer); t.index = len(*h); *h = append(*h, t) }
func (h *timerHeap) Pop() interface{} {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return t
}
