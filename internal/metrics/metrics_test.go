package metrics

import (
	"testing"
	"time"
)

func TestCounters(t *testing.T) {
	c := &Counters{}
	c.Added.Add(10)
	c.Fired.Add(5)
	snap := c.Take()
	if snap.Added != 10 || snap.Fired != 5 {
		t.Fatalf("unexpected: %+v", snap)
	}
}

func TestCountersReset(t *testing.T) {
	c := &Counters{}
	c.Added.Add(100)
	c.Reset()
	if c.Added.Load() != 0 {
		t.Fatal("expected 0")
	}
}

func TestLatencyTracker(t *testing.T) {
	lt := NewLatencyTracker(5)
	lt.Record(10 * time.Millisecond)
	lt.Record(20 * time.Millisecond)
	lt.Record(30 * time.Millisecond)
	if lt.Count() != 3 {
		t.Fatalf("expected 3, got %d", lt.Count())
	}
	if lt.Avg() != 20*time.Millisecond {
		t.Fatalf("avg=%v", lt.Avg())
	}
	if lt.Max() != 30*time.Millisecond {
		t.Fatalf("max=%v", lt.Max())
	}
}

func TestLatencyOverflow(t *testing.T) {
	lt := NewLatencyTracker(3)
	lt.Record(1 * time.Millisecond)
	lt.Record(2 * time.Millisecond)
	lt.Record(3 * time.Millisecond)
	lt.Record(4 * time.Millisecond) // evicts 1ms
	if lt.Count() != 3 {
		t.Fatalf("expected 3, got %d", lt.Count())
	}
}
