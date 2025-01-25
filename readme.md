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

| Method      | Description                                                                                                    |
|-------------|----------------------------------------------------------------------------------------------------------------|
| Wait        | Blocks until allowed by the limiter. Does not return anything                                                  |
| WaitTimout  | Blocks until the limiter allows or the timeout expires. Returns an error only if timeout expires.              |
| WaitContext | Blocks until the limiter allows or the context is canceled. Returns an error only if the context was canceled. |
| Allow       | Returns a boolean indicating if the operation is allowed by the limiter. It's non-blocking.                    |
| Clear       | Clears the limiter and returns to the initial state (does not wipe stat counters)                              |
| Stats       | Returns a struct with the current statistics of the limiter.                                                   |

Example usage:

```go

package main

import (
	"github.com/agustinbanchio/go-limit"
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
