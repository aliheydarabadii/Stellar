package ports

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Readiness struct {
	staleAfter   time.Duration
	lastSuccess  atomic.Int64
	hasSuccess   atomic.Bool
	shuttingDown atomic.Bool
}

func NewReadiness(staleAfter time.Duration) (*Readiness, error) {
	if staleAfter <= 0 {
		return nil, fmt.Errorf("readiness stale duration must be positive")
	}

	return &Readiness{staleAfter: staleAfter}, nil
}

func (r *Readiness) MarkSuccess(collectedAt time.Time) {
	if r == nil || collectedAt.IsZero() {
		return
	}

	r.lastSuccess.Store(collectedAt.UnixNano())
	r.hasSuccess.Store(true)
}

func (r *Readiness) MarkShuttingDown() {
	if r == nil {
		return
	}

	r.shuttingDown.Store(true)
}

func (r *Readiness) Ready(now time.Time) bool {
	if r == nil {
		return true
	}

	if r.shuttingDown.Load() || !r.hasSuccess.Load() {
		return false
	}

	lastSuccess := time.Unix(0, r.lastSuccess.Load())
	return now.Sub(lastSuccess) <= r.staleAfter
}
