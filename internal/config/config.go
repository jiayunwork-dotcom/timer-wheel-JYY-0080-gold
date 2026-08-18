// Package config provides configuration for the timer wheel.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrInvalid is returned for bad values.
var ErrInvalid = errors.New("config: invalid")

// Config holds timer wheel parameters.
type Config struct {
	// TickInterval is the base tick duration.
	TickInterval time.Duration `json:"tick_interval"`
	// WheelSize is the number of buckets per level.
	WheelSize int `json:"wheel_size"`
	// Workers is the callback worker count.
	Workers int `json:"workers"`
	// MaxTimers is the maximum pending timers.
	MaxTimers int `json:"max_timers"`
	// HeapThreshold: delays longer than this use the overflow heap.
	HeapThreshold time.Duration `json:"heap_threshold"`
}

// Default returns sensible defaults.
func Default() Config {
	return Config{
		TickInterval:  time.Millisecond,
		WheelSize:     256,
		Workers:       4,
		MaxTimers:     100000,
		HeapThreshold: time.Hour,
	}
}

// Validate checks constraints.
func (c *Config) Validate() error {
	if c.TickInterval <= 0 {
		return fmt.Errorf("%w: tick_interval must be positive", ErrInvalid)
	}
	if c.WheelSize <= 0 {
		return fmt.Errorf("%w: wheel_size must be positive", ErrInvalid)
	}
	if c.Workers <= 0 {
		return fmt.Errorf("%w: workers must be positive", ErrInvalid)
	}
	if c.MaxTimers <= 0 {
		return fmt.Errorf("%w: max_timers must be positive", ErrInvalid)
	}
	if c.HeapThreshold <= 0 {
		return fmt.Errorf("%w: heap_threshold must be positive", ErrInvalid)
	}
	return nil
}

// Save writes to dir/config.json.
func (c *Config) Save(dir string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

// Load reads from dir/config.json.
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
