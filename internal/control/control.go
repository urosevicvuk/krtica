package control

import (
	"context"
	"time"
)

type Heartbeat struct {
	Interval  time.Duration
	MaxMissed int
}

func (h *Heartbeat) Run(ctx context.Context) error {
	return nil
}

func (h *Heartbeat) RTT() time.Duration {
	return 0
}
