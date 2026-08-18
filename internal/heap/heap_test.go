package heap

import (
	"testing"
	"time"
)

func TestPushAndPop(t *testing.T) {
	h := New()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	h.Push(&Timer{ID: 1, Deadline: now.Add(5 * time.Second)})
	h.Push(&Timer{ID: 2, Deadline: now.Add(2 * time.Second)})
	h.Push(&Timer{ID: 3, Deadline: now.Add(8 * time.Second)})

	got := h.Pop()
	if got.ID != 2 {
		t.Fatalf("expected ID 2, got %d", got.ID)
	}
}

func TestPeek(t *testing.T) {
	h := New()
	now := time.Now()
	h.Push(&Timer{ID: 1, Deadline: now.Add(time.Second)})
	p := h.Peek()
	if p.ID != 1 {
		t.Fatal("unexpected peek")
	}
	if h.Len() != 1 {
		t.Fatal("peek should not remove")
	}
}

func TestRemove(t *testing.T) {
	h := New()
	now := time.Now()
	h.Push(&Timer{ID: 1, Deadline: now})
	h.Push(&Timer{ID: 2, Deadline: now})
	if !h.Remove(1) {
		t.Fatal("expected remove to succeed")
	}
	if h.Len() != 1 {
		t.Fatal("expected 1 remaining")
	}
}

func TestDrainExpired(t *testing.T) {
	h := New()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	h.Push(&Timer{ID: 1, Deadline: now.Add(-time.Second)})
	h.Push(&Timer{ID: 2, Deadline: now.Add(-time.Minute)})
	h.Push(&Timer{ID: 3, Deadline: now.Add(time.Hour)})

	expired := h.DrainExpired(now)
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(expired))
	}
	if h.Len() != 1 {
		t.Fatal("expected 1 remaining")
	}
}

func TestClear(t *testing.T) {
	h := New()
	h.Push(&Timer{ID: 1, Deadline: time.Now()})
	h.Clear()
	if h.Len() != 0 {
		t.Fatal("expected empty after clear")
	}
}
