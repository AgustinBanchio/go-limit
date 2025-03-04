package limit

import (
	"context"
	"errors"
	"fmt"
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

	// Reservation tracking
	pendingReservations map[*leakyBucketReservation]struct{}
}

func NewLeakyBucket(count int, duration time.Duration, maxQueue int) Limiter {
	leakRate := duration / time.Duration(count)
	return &leakyBucket{
		mux:                 sync.Mutex{},
		maxCapacity:         maxQueue,
		currentCapacity:     0,
		leakRate:            leakRate,
		lastLeak:            time.Now().Add(-leakRate),
		pendingReservations: make(map[*leakyBucketReservation]struct{}),
	}
}

func (l *leakyBucket) WaitContext(ctx context.Context) error {
	if l.currentCapacity+len(l.pendingReservations) >= l.maxCapacity {
		l.deniedEvents++
		return errors.New("max allowed queue reached")
	}

	l.currentCapacity++ // Queue the event
	for {
		l.mux.Lock()
		l.cleanupExpiredReservations()

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
			// Unqueue the event
			l.currentCapacity--
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

	// Mark all reservations as canceled
	for res := range l.pendingReservations {
		res.canceled = true
	}

	// Clear the pending reservations map
	l.pendingReservations = make(map[*leakyBucketReservation]struct{})

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

func (l *leakyBucket) Reserve() Reservation {
	reservation, _ := l.ReserveContext(context.Background())
	return reservation
}

func (l *leakyBucket) ReserveTimeout(timeout time.Duration) (Reservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return l.ReserveContext(ctx)
}

func (l *leakyBucket) ReserveContext(ctx context.Context) (Reservation, error) {
	var reservationDuration *time.Duration
	if deadline, ok := ctx.Deadline(); ok {
		reservationDuration = new(time.Duration)
		*reservationDuration = deadline.Sub(time.Now())
	}

	l.mux.Lock()
	l.cleanupExpiredReservations()

	if l.currentCapacity+len(l.pendingReservations) >= l.maxCapacity {
		l.deniedEvents++
		l.mux.Unlock()
		return nil, errors.New("max allowed queue reached")
	}

	var expiresAt *time.Time
	if reservationDuration != nil {
		expiresAt = new(time.Time)
		*expiresAt = time.Now().Add(*reservationDuration)
	}

	reservation := &leakyBucketReservation{
		limiter:   l,
		expiresAt: expiresAt,
	}
	l.pendingReservations[reservation] = struct{}{}
	l.mux.Unlock()

	return reservation, nil
}

func (l *leakyBucket) cleanupExpiredReservations() {
	// This must be called with the mutex already locked
	now := time.Now()
	for res := range l.pendingReservations {
		if res.expiresAt != nil && now.After(*res.expiresAt) {
			delete(l.pendingReservations, res)
		}
	}
}

// leakyBucketReservation implements the Reservation interface
type leakyBucketReservation struct {
	limiter   *leakyBucket
	expiresAt *time.Time
	consumed  bool
	canceled  bool
}

func (r *leakyBucketReservation) Consume() error {
	r.limiter.mux.Lock()

	if r.consumed {
		r.limiter.mux.Unlock()
		return fmt.Errorf("reservation already consumed")
	}

	if r.canceled {
		r.limiter.mux.Unlock()
		return fmt.Errorf("reservation was canceled")
	}

	if r.expiresAt != nil && time.Now().After(*r.expiresAt) {
		delete(r.limiter.pendingReservations, r)
		r.limiter.mux.Unlock()
		return fmt.Errorf("reservation expired")
	}

	r.consumed = true
	delete(r.limiter.pendingReservations, r)

	// In leaky bucket, consuming means adding to the current capacity queue
	r.limiter.currentCapacity++

	// Try to leak immediately
	if r.limiter.canLeak() {
		r.limiter.leak()
		r.limiter.allowedEvents++
		r.limiter.mux.Unlock()
		return nil
	}

	// We need to wait for leaking
	var deadline time.Time
	hasDeadline := false
	if r.expiresAt != nil {
		deadline = *r.expiresAt
		hasDeadline = true
	}

	r.limiter.mux.Unlock()

	// Wait for the event to be leaked
	for {
		// Calculate time to wait until next leak opportunity
		waitTime := r.limiter.lastLeak.Add(r.limiter.leakRate).Sub(time.Now())

		// If we have a deadline, ensure we don't wait past it
		if hasDeadline {
			timeToDeadline := deadline.Sub(time.Now())
			if timeToDeadline <= 0 {
				r.limiter.mux.Lock()
				// Don't decrement capacity as the event is still in queue
				r.limiter.mux.Unlock()
				return fmt.Errorf("reservation expired while waiting to leak")
			}

			// Use the shorter of the two wait times
			if timeToDeadline < waitTime {
				waitTime = timeToDeadline
			}
		}

		// Wait for the calculated time
		time.Sleep(waitTime)

		// Check if we can leak now
		r.limiter.mux.Lock()
		if r.limiter.canLeak() {
			r.limiter.leak()
			r.limiter.allowedEvents++
			r.limiter.mux.Unlock()
			return nil
		}
		r.limiter.mux.Unlock()
	}
}

func (r *leakyBucketReservation) Cancel() {
	r.limiter.mux.Lock()
	defer r.limiter.mux.Unlock()

	if !r.consumed {
		r.canceled = true
		delete(r.limiter.pendingReservations, r)
	}
}
