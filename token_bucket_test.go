package limit_test

import (
	"github.com/agustinbanchio/go-limit"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
		assert.True(t, limiter.Allow())
	}

	// Extra request should be denied
	assert.False(t, limiter.Allow())
}

// TODO: Add concurrency tests
