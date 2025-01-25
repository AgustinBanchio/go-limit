package limit_test

import (
	"github.com/agustinbanchio/go-limit"
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
	"time"
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
	assert.True(t, limiter.Allow())

	// 2nd request should be denied as we haven't waited for the minimum 200ms per request
	assert.False(t, limiter.Allow())

	// Wait for the minimum 200ms
	time.Sleep(200 * time.Millisecond)

	// 3rd request should be allowed
	assert.True(t, limiter.Allow())
}

// TODO: Add concurrency tests
