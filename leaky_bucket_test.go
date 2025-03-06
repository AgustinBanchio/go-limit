package limit_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/agustinbanchio/go-limit"
	"github.com/stretchr/testify/assert"
)

func TestLeakyBucket_Wait(t *testing.T) {
	t.Parallel()

	start := time.Now()

	// 5 requests per second
	limiter := limit.NewLeakyBucket(5, 1*time.Second, 100)

	limiter.Wait()
	log.Default().Print(time.Since(start))

	// First request should have been pseudo instant
	assert.True(t, time.Since(start) < time.Millisecond)

	for i := 0; i < 3; i++ {
		limiter.Wait()
		log.Default().Print(time.Since(start))
	}

	// The next 3 requests should have been spaced by 200ms each
	assert.True(t, time.Since(start) >= 600*time.Millisecond)
}

func TestLeakyBucket_Allow(t *testing.T) {
	t.Parallel()

	// 5 requests per second
	limiter := limit.NewLeakyBucket(5, 1*time.Second, 100)

	// 1st request should be allowed
	assert.True(t, limiter.Allowed())

	// 2nd request should be denied as we haven't waited for the minimum 200ms per request
	assert.False(t, limiter.Allowed())

	// Wait for the minimum 200ms
	time.Sleep(200 * time.Millisecond)

	// 3rd request should be allowed
	assert.True(t, limiter.Allowed())
}

func TestLeakyBucket_Reserve_BlocksAfterCapacityReached(t *testing.T) {
	t.Parallel()

	// 3 requests per second, max queue of 3
	limiter := limit.NewLeakyBucket(3, 1*time.Second, 3)

	// Make 3 reservations which should be immediate (filling the queue)
	var reservations []limit.Reservation
	start := time.Now()
	for i := 0; i < 3; i++ {
		res := limiter.Reserve(nil)
		reservations = append(reservations, res)
	}

	// All 3 reservations should have happened instantly
	assert.True(t, time.Since(start) < 50*time.Millisecond)

	// The fourth reservation should fail with max queue error
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := limiter.ReserveContext(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max allowed queue reached")

	// Verify reservations can be consumed
	// First consumption should be immediate, others should wait for leak rate
	for _, res := range reservations {
		assert.NoError(t, res.Consume())
	}
}
