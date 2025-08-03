// Package throttler provides a time-based throttling mechanism that limits
// the frequency of function execution. It ensures that a function is called
// at most once per specified duration, while collapsing multiple triggers
// within the same time window into a single execution.
//
// This is particularly useful for scenarios where you want to batch or
// debounce rapid changes, such as:
//   - File system watchers that trigger on multiple file changes
//   - UI updates that should be batched for better performance
//   - API calls that should be rate-limited
//   - Database writes that can be coalesced
//
// Example usage:
//
//	th := throttler.New(100 * time.Millisecond)
//	ctx := context.Background()
//
//	go th.Run(ctx, func() {
//		fmt.Println("This will be called at most once per 100ms")
//	})
//
//	// Multiple rapid triggers will be collapsed
//	th.Trigger()
//	th.Trigger()
//	th.Trigger()
package throttler

import (
	"context"
	"time"
)

// Throttler is a time-based throttling mechanism that limits function execution
// frequency. It uses a channel to receive trigger signals and a timer to
// enforce the minimum interval between executions.
//
// The zero value is not usable; use New() to create a properly initialized Throttler.
type Throttler struct {
	// C is the channel used to signal that a throttled action should be considered.
	// It has a buffer size of 1, which means multiple rapid triggers will be
	// collapsed into a single pending execution.
	C chan struct{}

	// Duration is the minimum time interval between function executions.
	// This creates a "cooling off" period where subsequent triggers are ignored.
	Duration time.Duration
}

// New creates a new Throttler with the specified minimum duration between
// function executions. The duration parameter determines how long the throttler
// will wait after executing the function before it can be executed again.
//
// Parameters:
//   - t: The minimum time.Duration between function executions
//
// Returns a properly initialized *Throttler ready for use.
//
// Example:
//
//	th := throttler.New(500 * time.Millisecond)
func New(t time.Duration) *Throttler {
	return &Throttler{
		C:        make(chan struct{}, 1),
		Duration: t,
	}
}

// Trigger signals that something has changed and the throttled function
// should be considered for execution. Multiple calls to Trigger() within
// the throttling window will be collapsed into a single execution.
//
// This method is non-blocking and safe for concurrent use. If the internal
// channel already has a pending trigger, subsequent triggers will be ignored
// until the pending one is processed.
//
// Trigger can be called from multiple goroutines safely.
//
// Example:
//
//	th.Trigger() // Will schedule execution
//	th.Trigger() // Will be ignored if previous trigger is still pending
func (t *Throttler) Trigger() {
	select {
	case t.C <- struct{}{}:
		// Successfully queued trigger
	default:
		// Channel full, trigger ignored (throttling in effect)
	}
}

// Run starts the throttler's main loop and will call fn at most once per Duration.
// The function will be executed at least once every Duration period if Trigger()
// is being called continuously.
//
// This method blocks until the provided context is cancelled. It should typically
// be run in its own goroutine.
//
// The throttling behavior works as follows:
//  1. When Trigger() is called, a pending flag is set
//  2. A timer is started for the configured Duration
//  3. When the timer expires, if there's a pending trigger, fn() is called
//  4. Multiple Trigger() calls during the timer period are collapsed into one execution
//  5. The timer is reset after each execution
//
// Parameters:
//   - ctx: Context for cancellation. When cancelled, Run() will return promptly.
//   - fn: The function to execute in a throttled manner. It should be relatively
//     quick to execute to avoid blocking the throttler's internal loop.
//
// Example:
//
//	th := throttler.New(100 * time.Millisecond)
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	go th.Run(ctx, func() {
//	    fmt.Println("Throttled execution")
//	})
//
//	// Trigger multiple times - will be collapsed
//	th.Trigger()
//	th.Trigger()
//	th.Trigger()
func (t *Throttler) Run(ctx context.Context, fn func()) {
	var (
		timer   *time.Timer
		timerCh <-chan time.Time
		pending bool
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-t.C:
			pending = true
			if timer == nil {
				timer = time.NewTimer(t.Duration)
				timerCh = timer.C
			}

		case <-timerCh:
			if pending {
				fn()
				pending = false
			}
			timer.Stop()
			timer = nil
			timerCh = nil
		}
	}
}
