package pool

import (
	"testing"
)

func TestSubmitAndDrain(t *testing.T) {
	p := New(4)
	count := 0
	p.Submit(Task{ID: 1, Fn: func() { count++ }})
	p.Submit(Task{ID: 2, Fn: func() { count++ }})
	n := p.Drain()
	if n != 2 || count != 2 {
		t.Fatalf("expected 2 executed, got n=%d count=%d", n, count)
	}
}

func TestPending(t *testing.T) {
	p := New(2)
	p.Submit(Task{ID: 1, Fn: func() {}})
	if p.Pending() != 1 {
		t.Fatalf("expected 1, got %d", p.Pending())
	}
	p.Drain()
	if p.Pending() != 0 {
		t.Fatal("expected 0 after drain")
	}
}

func TestClose(t *testing.T) {
	p := New(2)
	p.Close()
	err := p.Submit(Task{ID: 1, Fn: func() {}})
	if err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestExecuted(t *testing.T) {
	p := New(2)
	p.Submit(Task{ID: 1, Fn: func() {}})
	p.Submit(Task{ID: 2, Fn: func() {}})
	p.Drain()
	if p.Executed() != 2 {
		t.Fatalf("expected 2, got %d", p.Executed())
	}
}

func TestReset(t *testing.T) {
	p := New(2)
	p.Submit(Task{ID: 1, Fn: func() {}})
	p.Drain()
	p.Reset()
	if p.Executed() != 0 {
		t.Fatal("expected 0 after reset")
	}
}
