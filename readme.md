## GO-LIMIT

This module provides different in-memory rate limiter implementations for Go.

They are all concurrency safe, but currently do not respect FIFO order.

# Available Implementations

| Limiter                      | Description                                                                                           |
|------------------------------|-------------------------------------------------------------------------------------------------------|
| Rolling Window (Sliding Log) | The most accurate way to adhere to rate limits, uses more memory.                                     |
| Token Bucket                 | Uses the least memory, approximates the desired rate limit but might use slightly more during bursts. |
| Leaky Bucket                 | Distributes incoming events into steady flow.                                                         |

All implementations adhere to the same interface:

| Method         | Description                                                                                                                                   |
|----------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| Wait           | Blocks until allowed by the limiter. Does not return anything                                                                                 |
| WaitTimout     | Blocks until the limiter allows or the timeout expires. Returns an error only if timeout expires.                                             |
| WaitContext    | Blocks until the limiter allows or the context is canceled. Returns an error only if the context was canceled.                                |
| Allowed        | Returns a boolean indicating if the operation is allowed by the limiter. It's non-blocking.                                                   |
| Reserve        | Blocks until a reservation is returned by the limiter. Returns a Reservation that has the desired TTL.                                        |
| ReserveTimeout | Blocks until a reservation is returned by the limiter or the timeout expires. Returns a Reservation that has the desired TTL or an error.     |
| ReserveContext | Blocks until a reservation is returned by the limiter or the context is canceled. Returns a Reservation that has the desired TTL or an error. |
| Clear          | Clears the limiter and returns to the initial state (does not wipe stat counters)                                                             |
| Stats          | Returns a struct with the current statistics of the limiter.                                                                                  |

## Reservations

Reservations provide a way to reserve capacity without immediately consuming it:

| Method  | Description                                                         |
|---------|---------------------------------------------------------------------|
| Consume | Consumes the reserved token. Returns error if already used/expired. |
| Cancel  | Cancels the reservation, returning the token to the pool.           |

**Note:** The leaky bucket implementation provides only basic reservation functionality, which doesn't align perfectly
with the leaky bucket concept as it's primarily designed for rate smoothing rather than capacity reservation.

Reservations without TTL or not properly consumed or cancelled can lead to unused throughput or tokens being held
indefinitely.

Example usage:

```go

package main

import (
	"github.com/agustinbanchio/go-limit"
	"time"
)

func main() {

	limiter := limit.NewRollingWindow(1500, 1*time.Minute) // Don't allow more than 1500 requests in a 1-minute window.

	for i := 0; i < 1500; i++ {
		limiter.Wait()
	}
	// Those should be done almost instantly

	limiter.Wait() // This will block for almost a minute
}

```

## Purpose and Alternatives

This module is intended to provide an easy-to-work with interface for common rate limiting needs. Contributions and
suggestions are welcome, though implementation changes are not guaranteed.

If this interface or implementation doesn't meet your requirements, feel free to fork the project. Alternatively,
consider these excellent rate limiting libraries for Go:

[golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) - The extended standard library Go rate limiter
[uber-go/ratelimit](https://github.com/uber-go/ratelimit) - A leaky bucket rate limiter
