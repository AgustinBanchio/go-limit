package limit

import (
	"context"
	"time"
)

// Stats represents the current statistics of a rate limiter.
type Stats struct {
	// The total number of requests allowed since the limiter was created. Doesn't get reset when the limiter is cleared.
	AllowedRequests int
	// The total number of requests denied since the limiter was created. This includes requests that were waiting but timed out.
	DeniedRequests int
	// The time when the next request will be allowed.
	NextAllowedTime time.Time
}

// Limiter is the interface that wraps the basic methods of a rate limiter.
// Limiters should be safe for concurrent use by multiple goroutines.
type Limiter interface {
	// Wait blocks until the limiter allows the operation to proceed.
	Wait()
	// WaitTimeout blocks until the limiter allows the operation to proceed or the timeout expires.
	WaitTimeout(timeout time.Duration) error
	// WaitContext blocks until the limiter allows the operation to proceed or the context is done.
	WaitContext(ctx context.Context) error
	// Allowed returns true if the operation is allowed to proceed. It's non-blocking.
	Allowed() bool
	// Clear clears the limiter.
	Clear()
	// Stats returns the current stats of the limiter.
	Stats() Stats
	// Reserve blocks until the limiter can return a Reservation object. The Reservation has its own expiry duration or TTL. If nil it does not expire.
	Reserve(reservationTTL *time.Duration) Reservation
	// ReserveTimeout blocks until the limiter can return a Reservation object or the timeout expires. The Reservation has its own expiry duration or TTL. If nil it does not expire.
	ReserveTimeout(timeout time.Duration, reservationTTL *time.Duration) (Reservation, error)
	// ReserveContext requests a reservation with a context and returns a Reservation object.  The Reservation has its own expiry duration or TTL. If nil it does not expire. Context cancellation will only impact getting the reservation but will not expire the reservation itself.
	ReserveContext(ctx context.Context, reservationTTL *time.Duration) (Reservation, error)
}

// Reservation represents a reservation against a rate limiter that can be consumed or canceled
type Reservation interface {
	// Consume uses the reservation, returning an error if the reservation expired
	Consume() error
	// Cancel releases the reservation without using it
	Cancel()
}
