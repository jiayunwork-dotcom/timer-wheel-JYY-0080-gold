package wheel

import (
	"sort"
	"time"
)

// TimerInfo describes a pending timer.
type TimerInfo struct {
	ID       int64
	Deadline time.Time
	Priority int
}

// PendingTimers returns info about all pending timers, sorted by deadline.
func (w *Wheel) PendingTimers() []TimerInfo {
	w.mu.Lock()
	defer w.mu.Unlock()

	ids := w.PendingIDs()
	infos := make([]TimerInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, TimerInfo{ID: id})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
	return infos
}

// HasTimer reports whether a timer with the given ID is pending.
func (w *Wheel) HasTimer(id int64) bool {
	for _, pid := range w.PendingIDs() {
		if pid == id {
			return true
		}
	}
	return false
}

// CancelAll cancels all pending timers and returns how many were cancelled.
func (w *Wheel) CancelAll() int {
	ids := w.PendingIDs()
	count := 0
	for _, id := range ids {
		if w.Cancel(id) {
			count++
		}
	}
	return count
}
