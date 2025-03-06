package limit

import (
	"context"
	"fmt"
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
	allowedEvents       int
	deniedEvents        int
	rollingWindow       []eventLog
	pendingReservations map[*rollingWindowReservation]struct{} // Track actual reservation objects
}

// NewRollingWindow creates a new rolling window rate limiter.
// The count parameter is the number of events allowed in the duration.
// The duration parameter is the time window in which the events are allowed.
func NewRollingWindow(count int, duration time.Duration) Limiter {
	return &rollingWindow{
		mux:                 sync.Mutex{},
		maxEventCount:       count,
		rateDuration:        duration,
		rollingWindow:       make([]eventLog, 0),
		pendingReservations: make(map[*rollingWindowReservation]struct{}),
	}
}

func (r *rollingWindow) WaitContext(ctx context.Context) error {
	for {
		r.mux.Lock()
		r.removeExpiredEvents()
		r.cleanupExpiredReservations() // Clean up expired reservations

		if len(r.rollingWindow)+len(r.pendingReservations) < r.maxEventCount {
			r.rollingWindow = append(r.rollingWindow, eventLog{timestamp: time.Now()})
			r.allowedEvents++
			r.mux.Unlock()
			return nil
		}

		r.mux.Unlock()

		waitDuration := r.rateDuration
		if len(r.rollingWindow) > 0 {
			waitDuration = r.rollingWindow[0].timestamp.Add(r.rateDuration).Sub(time.Now())
		}
		select {
		case <-ctx.Done():
			r.mux.Lock()
			r.deniedEvents++
			r.mux.Unlock()
			return ctx.Err()
		case <-time.After(waitDuration):
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

func (r *rollingWindow) Allowed() bool {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.removeExpiredEvents()
	r.cleanupExpiredReservations() // Clean up expired reservations

	// Check considering both active events and pending reservations
	if len(r.rollingWindow)+len(r.pendingReservations) < r.maxEventCount {
		r.rollingWindow = append(r.rollingWindow, eventLog{timestamp: time.Now()})
		r.allowedEvents++
		return true
	}

	r.deniedEvents++
	return false
}

func (r *rollingWindow) removeExpiredEvents() {
	// This must be called with the mutex already locked
	for len(r.rollingWindow) > 0 && time.Since(r.rollingWindow[0].timestamp) > r.rateDuration {
		r.rollingWindow = r.rollingWindow[1:]
	}
}

func (r *rollingWindow) cleanupExpiredReservations() {
	// This must be called with the mutex already locked
	now := time.Now()
	for res := range r.pendingReservations {
		if res.expiresAt != nil && now.After(*res.expiresAt) {
			delete(r.pendingReservations, res)
		}
	}
}

func (r *rollingWindow) Clear() {
	r.mux.Lock()
	defer r.mux.Unlock()

	// Mark all reservations as canceled
	for res := range r.pendingReservations {
		res.canceled = true
	}

	// Clear the pending reservations map
	r.pendingReservations = make(map[*rollingWindowReservation]struct{})

	// Clear the rolling window
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

func (r *rollingWindow) Reserve(reservationTTL *time.Duration) Reservation {
	reservation, _ := r.ReserveContext(context.Background(), reservationTTL)
	return reservation
}

func (r *rollingWindow) ReserveTimeout(timeout time.Duration, reservationTTL *time.Duration) (Reservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.ReserveContext(ctx, reservationTTL)
}

func (r *rollingWindow) ReserveContext(ctx context.Context, reservationTTL *time.Duration) (Reservation, error) {
	for {
		r.mux.Lock()
		r.removeExpiredEvents()
		r.cleanupExpiredReservations() // Clean up expired reservations

		// Consider both actual events and pending reservations
		if len(r.rollingWindow)+len(r.pendingReservations) < r.maxEventCount {
			var expiresAt *time.Time
			if reservationTTL != nil {
				expiresAt = new(time.Time)
				*expiresAt = time.Now().Add(*reservationTTL)
			}
			reservation := &rollingWindowReservation{
				limiter:   r,
				expiresAt: expiresAt, // Expires after same time as wait time
			}
			r.pendingReservations[reservation] = struct{}{} // Track this reservation
			r.mux.Unlock()
			return reservation, nil
		}

		waitDuration := r.rateDuration
		if len(r.rollingWindow) > 0 {
			waitDuration = r.rollingWindow[0].timestamp.Add(r.rateDuration).Sub(time.Now())
		}
		r.mux.Unlock()

		select {
		case <-ctx.Done():
			r.mux.Lock()
			r.deniedEvents++
			r.mux.Unlock()
			return nil, ctx.Err()
		case <-time.After(waitDuration):
			// Continue waiting
		}
	}
}

// rollingWindowReservation implements the Reservation interface
type rollingWindowReservation struct {
	limiter   *rollingWindow
	expiresAt *time.Time
	consumed  bool
	canceled  bool
}

func (r *rollingWindowReservation) Consume() error {
	r.limiter.mux.Lock()
	defer r.limiter.mux.Unlock()

	if r.consumed {
		return fmt.Errorf("reservation already consumed")
	}

	if r.canceled {
		return fmt.Errorf("reservation was canceled")
	}

	if r.expiresAt != nil && time.Now().After(*r.expiresAt) {
		delete(r.limiter.pendingReservations, r) // Remove expired reservation
		return fmt.Errorf("reservation expired")
	}

	r.consumed = true
	delete(r.limiter.pendingReservations, r) // Remove from pending
	r.limiter.rollingWindow = append(r.limiter.rollingWindow, eventLog{timestamp: time.Now()})
	r.limiter.allowedEvents++

	return nil
}

func (r *rollingWindowReservation) Cancel() {
	r.limiter.mux.Lock()
	defer r.limiter.mux.Unlock()

	if !r.consumed {
		r.canceled = true
		delete(r.limiter.pendingReservations, r) // Remove from pending
	}
}
