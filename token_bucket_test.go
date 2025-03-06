package limit_test

import (
	"context"
	"testing"
	"time"

	"github.com/agustinbanchio/go-limit"
	"github.com/stretchr/testify/assert"
)

func TestTokenBucket_Wait(t *testing.T) {
	t.Parallel()

	start := time.Now()

	// 5 requests per second
	limiter := limit.NewTokenBucket(5, 1*time.Second)
	for i := 0; i < 5; i++ {
		limiter.Wait()
	}

	// 5 requests should have been pseudo instant
	assert.True(t, time.Since(start) < time.Millisecond)

	// Extra request should wait for the refill rate (approx 200ms
	limiter.Wait()
	assert.True(t, time.Since(start) >= 200*time.Millisecond)
}

func TestTokenBucket_Allow(t *testing.T) {
	t.Parallel()

	// 5 requests per second
	limiter := limit.NewTokenBucket(5, 1*time.Second)

	// 5 requests should be allowed
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allowed())
	}

	// Extra request should be denied
	assert.False(t, limiter.Allowed())
}

func TestTokenBucket_Reserve_BlocksAfterCapacityReached(t *testing.T) {
	t.Parallel()

	// 3 requests per second
	limiter := limit.NewTokenBucket(3, 1*time.Second)

	// Make 3 reservations which should be immediate
	var reservations []limit.Reservation
	start := time.Now()
	for i := 0; i < 3; i++ {
		res := limiter.Reserve(nil)
		reservations = append(reservations, res)
	}

	// All 3 reservations should have happened instantly
	assert.True(t, time.Since(start) < 50*time.Millisecond)

	// The fourth reservation should block, so use a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := limiter.ReserveContext(ctx, nil)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)

	// Verify reservations can be consumed
	for _, res := range reservations {
		assert.NoError(t, res.Consume())
	}
}

func TestTokenBucket_Reserve_CancelFreesCapacity(t *testing.T) {
	t.Parallel()

	// 2 requests per second
	limiter := limit.NewTokenBucket(2, 1*time.Second)

	// Make 2 reservations, filling capacity
	res1 := limiter.Reserve(nil)
	res2 := limiter.Reserve(nil)

	// At this point, Allowed should fail because we're at capacity
	assert.False(t, limiter.Allowed())

	// Cancel one reservation
	res1.Cancel()

	// Now Allowed should succeed
	assert.True(t, limiter.Allowed())

	// The remaining reservation should still be consumable
	assert.NoError(t, res2.Consume())

	// After consuming, we should be at capacity again
	assert.False(t, limiter.Allowed())
}
