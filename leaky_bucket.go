package limit

import (
	"context"
	"errors"
	"sync"
	"time"
)

type leakyBucket struct {
	// Mutex
	mux sync.Mutex

	// Config
	maxCapacity     int
	currentCapacity int // Queued events
	leakRate        time.Duration

	// State
	allowedEvents int
	deniedEvents  int

	lastLeak time.Time
}

func NewLeakyBucket(count int, duration time.Duration, maxQueue int) Limiter {
	leakRate := duration / time.Duration(count)
	return &leakyBucket{
		mux:             sync.Mutex{},
		maxCapacity:     maxQueue,
		currentCapacity: 0,
		leakRate:        leakRate,
		lastLeak:        time.Now().Add(-leakRate),
	}
}

func (l *leakyBucket) WaitContext(ctx context.Context) error {
	if l.currentCapacity >= l.maxCapacity {
		l.deniedEvents++
		return errors.New("max allowed queue reached")
	}

	l.currentCapacity++ // Queue the event
	for {
		l.mux.Lock()

		if l.canLeak() {
			l.leak()
			l.allowedEvents++
			l.mux.Unlock()
			return nil
		}

		l.mux.Unlock()

		select {
		case <-ctx.Done():
			l.mux.Lock()
			l.deniedEvents++
			l.mux.Unlock()
			return ctx.Err()
		case <-time.After(l.lastLeak.Add(l.leakRate).Sub(time.Now())):
			// Wait until the next event is allowed
		}
	}
}

func (l *leakyBucket) Wait() {
	_ = l.WaitContext(context.Background())
}

func (l *leakyBucket) WaitTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return l.WaitContext(ctx)
}

// Allow does not increase capacity as it does not wait.
func (l *leakyBucket) Allow() bool {
	l.mux.Lock()
	defer l.mux.Unlock()

	if l.currentCapacity == 0 && l.canLeak() {
		l.leak()
		l.allowedEvents++
		return true
	}

	l.deniedEvents++
	return false
}

func (l *leakyBucket) canLeak() bool {
	return time.Since(l.lastLeak) >= l.leakRate
}

func (l *leakyBucket) leak() {
	l.currentCapacity--
	if l.currentCapacity < 0 {
		l.currentCapacity = 0
	}
	l.lastLeak = time.Now()
}

func (l *leakyBucket) Clear() {
	l.mux.Lock()
	defer l.mux.Unlock()
	l.currentCapacity = 0
	l.lastLeak = time.Now().Add(-l.leakRate)
}

func (l *leakyBucket) Stats() Stats {
	l.mux.Lock()
	defer l.mux.Unlock()

	nextAllowedTime := time.Now()
	if l.currentCapacity > 0 {
		nextAllowedTime = l.lastLeak.Add(l.leakRate)
	}

	return Stats{
		AllowedRequests: l.allowedEvents,
		DeniedRequests:  l.deniedEvents,
		NextAllowedTime: nextAllowedTime,
	}
}
