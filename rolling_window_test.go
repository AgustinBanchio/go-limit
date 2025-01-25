package rlimit_test

import (
	"github.com/agustinbanchio/rlimit"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRollingWindow_Wait(t *testing.T) {
	t.Parallel()

	// 5 requests per second
	limiter := rlimit.NewRollingWindow(5, 1*time.Second)

	start := time.Now()
	for i := 0; i < 5; i++ {
		limiter.Wait()
	}

	// 5 requests should have been pseudo instant
	assert.True(t, time.Since(start) < time.Microsecond)

	// Extra request should wait for the next 1 second
	limiter.Wait()
	assert.True(t, time.Since(start) >= time.Second)
}

func TestRollingWindow_Allow(t *testing.T) {
	t.Parallel()

	// 5 requests per second
	limiter := rlimit.NewRollingWindow(5, 1*time.Second)

	// 5 requests should be allowed
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow())
	}

	// Extra request should be denied
	assert.False(t, limiter.Allow())
}

// TODO: Add concurrency tests
