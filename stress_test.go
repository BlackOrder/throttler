package throttler

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestExtremeConcurrency tests the throttler under extreme concurrent load
func TestExtremeConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping extreme concurrency test in short mode")
	}

	th := New(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var callCount int64
	var triggerCount int64

	fn := func() {
		atomic.AddInt64(&callCount, 1)
	}

	// Start the throttler
	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Launch an extreme number of goroutines
	numGoroutines := 1000
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine triggers very rapidly
			for j := 0; j < 500; j++ {
				th.Trigger()
				atomic.AddInt64(&triggerCount, 1)

				// Occasionally yield to increase scheduling pressure
				if j%50 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}

	wg.Wait()
	<-done

	finalCallCount := atomic.LoadInt64(&callCount)
	finalTriggerCount := atomic.LoadInt64(&triggerCount)

	if finalCallCount < 1 {
		t.Errorf("Expected at least 1 function call, got %d", finalCallCount)
	}

	compressionRatio := float64(finalTriggerCount) / float64(finalCallCount)
	t.Logf("Extreme concurrency test: %d triggers -> %d calls (compression ratio: %.1fx)",
		finalTriggerCount, finalCallCount, compressionRatio)

	if compressionRatio < 100 {
		t.Logf("Warning: Lower than expected compression ratio, might indicate throttling issues")
	}
}

// TestHighFrequencyTriggers tests very high frequency triggering
func TestHighFrequencyTriggers(t *testing.T) {
	th := New(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var callCount int64
	var triggerCount int64

	fn := func() {
		atomic.AddInt64(&callCount, 1)
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Single goroutine triggering at maximum speed
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				th.Trigger()
				atomic.AddInt64(&triggerCount, 1)
			}
		}
	}()

	<-done

	finalCallCount := atomic.LoadInt64(&callCount)
	finalTriggerCount := atomic.LoadInt64(&triggerCount)

	t.Logf("High frequency test: %d triggers -> %d calls in 500ms",
		finalTriggerCount, finalCallCount)

	if finalCallCount < 5 {
		t.Errorf("Expected at least 5 calls in 500ms with 20ms throttling, got %d", finalCallCount)
	}

	if finalTriggerCount < 1000 {
		t.Errorf("Expected high trigger rate, got only %d triggers", finalTriggerCount)
	}
}

// TestLongRunningFunction tests throttling with long-running functions
func TestLongRunningFunction(t *testing.T) {
	th := New(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var callCount int64
	var functionRunning int64

	fn := func() {
		// Check if another instance is already running (should never happen)
		if !atomic.CompareAndSwapInt64(&functionRunning, 0, 1) {
			t.Errorf("Function called while another instance was running!")
			return
		}

		atomic.AddInt64(&callCount, 1)

		// Simulate long-running work
		time.Sleep(30 * time.Millisecond)

		atomic.StoreInt64(&functionRunning, 0)
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Trigger repeatedly during the function execution
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				th.Trigger()
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	<-done

	finalCallCount := atomic.LoadInt64(&callCount)
	t.Logf("Long-running function test: %d calls", finalCallCount)

	// Should have multiple calls but not too many due to long execution time
	if finalCallCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCallCount)
	}
	if finalCallCount > 10 {
		t.Errorf("Too many calls for long-running function: %d", finalCallCount)
	}
}

// TestRaceConditionEdgeCases tests specific edge cases that might cause races
func TestRaceConditionEdgeCases(t *testing.T) {
	// Test rapid creation and triggering
	for i := 0; i < 100; i++ {
		func() {
			th := New(1 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			var called bool
			done := make(chan bool)

			go func() {
				th.Run(ctx, func() {
					called = true
				})
				done <- true
			}()

			// Trigger immediately after starting
			th.Trigger()

			<-done

			if !called {
				t.Errorf("Function was not called in iteration %d", i)
			}
		}()
	}
}

// TestTimerResetRace tests for races in timer reset logic
func TestTimerResetRace(t *testing.T) {
	th := New(25 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var callCount int64

	fn := func() {
		count := atomic.AddInt64(&callCount, 1)
		t.Logf("Function call %d", count)
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Create a pattern that might cause timer reset races
	for i := 0; i < 5; i++ {
		// Trigger, wait slightly less than the timer, trigger again
		th.Trigger()
		time.Sleep(20 * time.Millisecond)
		th.Trigger()
		time.Sleep(30 * time.Millisecond) // Let timer expire
	}

	<-done

	finalCallCount := atomic.LoadInt64(&callCount)
	if finalCallCount < 3 {
		t.Errorf("Expected at least 3 calls, got %d", finalCallCount)
	}

	t.Logf("Timer reset race test: %d calls", finalCallCount)
}

// TestConcurrentContextCancellation tests cancelling context while triggers are happening
func TestConcurrentContextCancellation(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		func() {
			th := New(10 * time.Millisecond)
			ctx, cancel := context.WithCancel(context.Background())

			var callCount int64

			done := make(chan bool)
			go func() {
				th.Run(ctx, func() {
					atomic.AddInt64(&callCount, 1)
				})
				done <- true
			}()

			var wg sync.WaitGroup

			// Start triggering goroutines
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						select {
						case <-ctx.Done():
							return
						default:
							th.Trigger()
						}
					}
				}()
			}

			// Cancel after a random short time
			time.Sleep(time.Duration(iteration%10+1) * time.Millisecond)
			cancel()

			// Wait for triggers to stop
			wg.Wait()

			// Wait for Run to exit
			select {
			case <-done:
				// Expected
			case <-time.After(50 * time.Millisecond):
				t.Errorf("Run() did not exit promptly in iteration %d", iteration)
			}

			finalCount := atomic.LoadInt64(&callCount)
			t.Logf("Iteration %d: %d calls before cancellation", iteration, finalCount)
		}()
	}
}
