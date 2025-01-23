package rlmit

import (
	"context"
	"sync"
	"time"
)

type eventLog struct {
	timestamp time.Time
}

type rollingWindow struct {
	// Mutex
	mux sync.Mutex

	// Config
	maxEventCount int
	rateDuration  time.Duration

	// State
	allowedEvents int
	deniedEvents  int
	rollingWindow []eventLog
}

// NewRollingWindow creates a new rolling window rate limiter.
// The count parameter is the number of events allowed in the duration.
// The duration parameter is the time window in which the events are allowed.
func NewRollingWindow(count int, duration time.Duration) Limiter {
	return &rollingWindow{
		mux:           sync.Mutex{},
		maxEventCount: count,
		rateDuration:  duration,
		rollingWindow: make([]eventLog, 0),
	}
}

func (r *rollingWindow) WaitContext(ctx context.Context) error {
	for {
		r.mux.Lock()
		r.removeExpiredEvents()

		if len(r.rollingWindow) < r.maxEventCount {
			r.rollingWindow = append(r.rollingWindow, eventLog{timestamp: time.Now()})
			r.allowedEvents++
			r.mux.Unlock()
			return nil
		}

		r.mux.Unlock()

		select {
		case <-ctx.Done():
			r.mux.Lock()
			r.deniedEvents++
			r.mux.Unlock()
			return ctx.Err()
		case <-time.After(r.rollingWindow[0].timestamp.Add(r.rateDuration).Sub(time.Now())):
			// Wait until the next event is allowed
		}
	}
}

func (r *rollingWindow) Wait() {
	_ = r.WaitContext(context.Background())
}

func (r *rollingWindow) WaitTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.WaitContext(ctx)
}

func (r *rollingWindow) Allow() bool {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.removeExpiredEvents()

	// Check if the event can be allowed
	if len(r.rollingWindow) < r.maxEventCount {
		r.rollingWindow = append(r.rollingWindow, eventLog{timestamp: time.Now()})
		r.allowedEvents++
		return true
	}

	r.deniedEvents++
	return false
}

func (r *rollingWindow) removeExpiredEvents() {
	for len(r.rollingWindow) > 0 && time.Since(r.rollingWindow[0].timestamp) > r.rateDuration {
		r.rollingWindow = r.rollingWindow[1:]
	}
}

func (r *rollingWindow) Clear() {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.rollingWindow = make([]eventLog, 0)
}

func (r *rollingWindow) Stats() Stats {
	r.mux.Lock()
	defer r.mux.Unlock()

	nextAllowedTime := time.Now()
	if len(r.rollingWindow) > 0 {
		nextAllowedTime = r.rollingWindow[0].timestamp.Add(r.rateDuration)
	}

	return Stats{
		AllowedRequests: r.allowedEvents,
		DeniedRequests:  r.deniedEvents,
		NextAllowedTime: nextAllowedTime,
	}
}
