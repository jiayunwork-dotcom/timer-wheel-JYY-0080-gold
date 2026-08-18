package wheel

import (
	"container/heap"
)

// bucketHeap is a min-heap of buckets ordered by their expiration. Only the
// push/pop operations are used, so buckets never need to be removed from the
// middle of the heap.
type bucketHeap []*bucket

func (h bucketHeap) Len() int            { return len(h) }
func (h bucketHeap) Less(i, j int) bool  { return h[i].expiration.Before(h[j].expiration) }
func (h bucketHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *bucketHeap) Push(x any)         { *h = append(*h, x.(*bucket)) }
func (h *bucketHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return it
}
func (h bucketHeap) peek() *bucket {
	if len(h) == 0 {
		return nil
	}
	return h[0]
}

// entryHeap is a min-heap of entries ordered by their expiration. It backs the
// "due within a tick" queue so fired tasks come out in chronological order.
type entryHeap []*entry

func (h entryHeap) Len() int           { return len(h) }
func (h entryHeap) Less(i, j int) bool {
	// Earlier deadline fires first.
	if !h[i].expiration.Equal(h[j].expiration) {
		return h[i].expiration.Before(h[j].expiration)
	}
	// Tie on deadline: higher priority fires first, then lower id for
	// determinism.
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	return h[i].ID < h[j].ID
}
func (h entryHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *entryHeap) Push(x any)         { *h = append(*h, x.(*entry)) }
func (h *entryHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return it
}
func (h entryHeap) peek() *entry {
	if len(h) == 0 {
		return nil
	}
	return h[0]
}

func heapInit(h heap.Interface) { heap.Init(h) }
func heapPush(h heap.Interface, x any) {
	heap.Push(h, x)
}
func heapPop(h heap.Interface) any {
	return heap.Pop(h)
}
