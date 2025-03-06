package limit

import (
	"context"
	"fmt"
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

	// Reservations tracking
	pendingReservations map[*tokenBucketReservation]struct{}
}

func NewTokenBucket(count int, duration time.Duration) Limiter {
	return &tokenBucket{
		mux:                 sync.Mutex{},
		maxCapacity:         count,
		currentCapacity:     count,
		refillRate:          duration / time.Duration(count),
		lastRefill:          time.Now(),
		pendingReservations: make(map[*tokenBucketReservation]struct{}),
	}
}

func (t *tokenBucket) WaitContext(ctx context.Context) error {
	for {
		t.mux.Lock()
		t.refill()
		t.cleanupExpiredReservations()

		if t.currentCapacity-len(t.pendingReservations) > 0 {
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

func (t *tokenBucket) Allowed() bool {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.refill()
	t.cleanupExpiredReservations()

	if t.currentCapacity-len(t.pendingReservations) > 0 {
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

	// Mark all reservations as canceled
	for res := range t.pendingReservations {
		res.canceled = true
	}

	// Clear the pending reservations map
	t.pendingReservations = make(map[*tokenBucketReservation]struct{})
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

func (t *tokenBucket) cleanupExpiredReservations() {
	// This must be called with the mutex already locked
	now := time.Now()
	for res := range t.pendingReservations {
		if res.expiresAt != nil && now.After(*res.expiresAt) {
			delete(t.pendingReservations, res)
		}
	}
}

func (t *tokenBucket) Reserve(reservationTTL *time.Duration) Reservation {
	reservation, _ := t.ReserveContext(context.Background(), reservationTTL)
	return reservation
}

func (t *tokenBucket) ReserveTimeout(timeout time.Duration, reservationTTL *time.Duration) (Reservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return t.ReserveContext(ctx, reservationTTL)
}

func (t *tokenBucket) ReserveContext(ctx context.Context, reservationTTL *time.Duration) (Reservation, error) {
	for {
		t.mux.Lock()
		t.refill()
		t.cleanupExpiredReservations()

		if t.currentCapacity-len(t.pendingReservations) > 0 {
			var expiresAt *time.Time
			if reservationTTL != nil {
				expiresAt = new(time.Time)
				*expiresAt = time.Now().Add(*reservationTTL)
			}
			reservation := &tokenBucketReservation{
				limiter:   t,
				expiresAt: expiresAt,
			}
			t.pendingReservations[reservation] = struct{}{}
			t.mux.Unlock()
			return reservation, nil
		}

		nextRefillTime := t.lastRefill.Add(t.refillRate)
		t.mux.Unlock()

		select {
		case <-ctx.Done():
			t.mux.Lock()
			t.deniedEvents++
			t.mux.Unlock()
			return nil, ctx.Err()
		case <-time.After(nextRefillTime.Sub(time.Now())):
			// Continue waiting for a token
		}
	}
}

// tokenBucketReservation implements the Reservation interface
type tokenBucketReservation struct {
	limiter   *tokenBucket
	expiresAt *time.Time
	consumed  bool
	canceled  bool
}

func (r *tokenBucketReservation) Consume() error {
	r.limiter.mux.Lock()
	defer r.limiter.mux.Unlock()

	if r.consumed {
		return fmt.Errorf("reservation already consumed")
	}

	if r.canceled {
		return fmt.Errorf("reservation was canceled")
	}

	if r.expiresAt != nil && time.Now().After(*r.expiresAt) {
		delete(r.limiter.pendingReservations, r)
		return fmt.Errorf("reservation expired")
	}

	r.consumed = true
	delete(r.limiter.pendingReservations, r)
	// Only decrease capacity when actually consumed
	r.limiter.currentCapacity--
	r.limiter.allowedEvents++

	return nil
}

func (r *tokenBucketReservation) Cancel() {
	r.limiter.mux.Lock()
	defer r.limiter.mux.Unlock()

	if !r.consumed {
		r.canceled = true
		delete(r.limiter.pendingReservations, r)
	}
}
