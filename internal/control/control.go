package control

import (
	"context"
	"time"
)

// Heartbeat is a control API heartbeat
type Heartbeat struct {
	Interval  time.Duration
	MaxMissed int
}

// Run runs the heartbeat
func (h *Heartbeat) Run(ctx context.Context) error {
	return nil
}

// RTT returns the current RTT
func (h *Heartbeat) RTT() time.Duration {
	return 0
}
