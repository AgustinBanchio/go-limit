package rlimit

import (
	"context"
	"sync"
	"time"
)

type tokenBucket struct {
	// Mutex
	mux sync.Mutex

	// Config
	maxCapacity     int
	currentCapacity int
	refillRate      time.Duration

	// State
	allowedEvents int
	deniedEvents  int
	lastRefill    time.Time
}

func NewTokenBucket(count int, duration time.Duration) Limiter {
	return &tokenBucket{
		mux:             sync.Mutex{},
		maxCapacity:     count,
		currentCapacity: count,
		refillRate:      duration / time.Duration(count),
		lastRefill:      time.Now(),
	}
}

func (t *tokenBucket) WaitContext(ctx context.Context) error {
	for {
		t.mux.Lock()
		t.refill()

		if t.currentCapacity > 0 {
			t.currentCapacity--
			t.allowedEvents++
			t.mux.Unlock()
			return nil
		}

		t.mux.Unlock()

		select {
		case <-ctx.Done():
			t.mux.Lock()
			t.deniedEvents++
			t.mux.Unlock()
			return ctx.Err()
		case <-time.After(t.lastRefill.Add(t.refillRate).Sub(time.Now())):
			// Wait until the next event is allowed
		}
	}
}

func (t *tokenBucket) Wait() {
	_ = t.WaitContext(context.Background())
}

func (t *tokenBucket) WaitTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return t.WaitContext(ctx)
}

func (t *tokenBucket) Allow() bool {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.refill()

	if t.currentCapacity > 0 {
		t.currentCapacity--
		t.allowedEvents++
		return true
	}

	t.deniedEvents++
	return false
}

func (t *tokenBucket) Clear() {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.currentCapacity = t.maxCapacity
	t.lastRefill = time.Now()
}

func (t *tokenBucket) Stats() Stats {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.refill()
	nextAllowedTime := time.Now()
	if t.currentCapacity == 0 {
		nextAllowedTime = t.lastRefill.Add(t.refillRate)
	}

	return Stats{
		AllowedRequests: t.allowedEvents,
		DeniedRequests:  t.deniedEvents,
		NextAllowedTime: nextAllowedTime,
	}
}

func (t *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(t.lastRefill)
	newTokens := int(elapsed / t.refillRate)

	if newTokens == 0 {
		return
	}

	if t.currentCapacity+newTokens > t.maxCapacity {
		t.currentCapacity = t.maxCapacity
	} else {
		t.currentCapacity += newTokens
	}
	t.lastRefill = now
}
