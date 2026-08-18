package wheel

import "time"

// Stats holds diagnostic info about the wheel state.
type Stats struct {
	Pending     int
	WheelSize   int
	TickSize    time.Duration
	CurrentTime time.Time
}

// ComputeStats returns current wheel statistics.
func (w *Wheel) ComputeStats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Stats{
		Pending:     w.Len(),
		WheelSize:   w.wheelSize,
		TickSize:    w.tick,
		CurrentTime: w.currentTime,
	}
}

// Interval returns the total interval covered by one full rotation.
func (w *Wheel) Interval() time.Duration {
	return w.interval
}

// CurrentTime returns the wheel's current logical time.
func (w *Wheel) CurrentTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentTime
}

// WheelSize returns the number of buckets.
func (w *Wheel) WheelSize() int {
	return w.wheelSize
}
