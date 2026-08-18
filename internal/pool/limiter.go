package pool

import (
	"sync"
	"time"
)

// RateLimiter limits the rate of task execution.
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	tokens   int
	maxBurst int
	lastFill time.Time
}

// NewRateLimiter creates a limiter that allows maxBurst tasks per interval.
func NewRateLimiter(interval time.Duration, maxBurst int) *RateLimiter {
	if maxBurst <= 0 {
		maxBurst = 1
	}
	return &RateLimiter{
		interval: interval,
		tokens:   maxBurst,
		maxBurst: maxBurst,
		lastFill: time.Now(),
	}
}

// Allow reports whether a task may execute. Consumes one token.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

// AllowN reports whether n tasks may execute.
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	if rl.tokens >= n {
		rl.tokens -= n
		return true
	}
	return false
}

// Tokens returns current available tokens.
func (rl *RateLimiter) Tokens() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	return rl.tokens
}

// Reset restores tokens to max.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.tokens = rl.maxBurst
	rl.lastFill = time.Now()
}

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastFill)
	if elapsed >= rl.interval {
		fills := int(elapsed / rl.interval)
		rl.tokens += fills
		if rl.tokens > rl.maxBurst {
			rl.tokens = rl.maxBurst
		}
		rl.lastFill = rl.lastFill.Add(time.Duration(fills) * rl.interval)
	}
}
