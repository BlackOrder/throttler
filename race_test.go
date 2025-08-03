package throttler

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestConcurrentTriggers tests multiple goroutines calling Trigger() simultaneously
func TestConcurrentTriggers(t *testing.T) {
	th := New(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	// Start the throttler
	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Number of goroutines to spawn
	numGoroutines := 100
	var wg sync.WaitGroup

	// Launch multiple goroutines that trigger rapidly
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine triggers multiple times
			for j := 0; j < 10; j++ {
				th.Trigger()
				// Small random delays to create more scheduling variations
				if j%3 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}

	wg.Wait()
	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have been called at least once, but not too many times due to throttling
	if finalCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCount)
	}
	if finalCount > 10 {
		t.Errorf("Expected reasonable throttling, got %d calls", finalCount)
	}

	t.Logf("Function called %d times with %d goroutines triggering", finalCount, numGoroutines)
}

// TestConcurrentTriggersWithLongRunning tests concurrent triggers with a longer-running function
func TestConcurrentTriggersWithLongRunning(t *testing.T) {
	th := New(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		// Simulate some work
		time.Sleep(10 * time.Millisecond)

		t.Logf("Function execution %d completed", current)
	}

	// Start the throttler
	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Launch goroutines that trigger continuously
	numGoroutines := 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Trigger every few milliseconds
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()

			triggerCount := 0
			for {
				select {
				case <-ticker.C:
					th.Trigger()
					triggerCount++
					if triggerCount >= 20 {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}

	wg.Wait()
	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	if finalCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCount)
	}

	t.Logf("Function called %d times with continuous triggering from %d goroutines", finalCount, numGoroutines)
}

// TestMultipleThrottlers tests multiple independent throttlers running concurrently
func TestMultipleThrottlers(t *testing.T) {
	numThrottlers := 10
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var totalCalls int
	var mu sync.Mutex

	var wg sync.WaitGroup

	for i := 0; i < numThrottlers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			th := New(50 * time.Millisecond)
			var localCalls int

			localDone := make(chan bool)
			go func() {
				th.Run(ctx, func() {
					localCalls++
					mu.Lock()
					totalCalls++
					mu.Unlock()
				})
				localDone <- true
			}()

			// Trigger this throttler multiple times
			for j := 0; j < 20; j++ {
				th.Trigger()
				time.Sleep(5 * time.Millisecond)
			}

			<-localDone
			t.Logf("Throttler %d: %d calls", id, localCalls)
		}(i)
	}

	wg.Wait()

	mu.Lock()
	finalTotal := totalCalls
	mu.Unlock()

	if finalTotal < numThrottlers {
		t.Errorf("Expected at least %d calls (one per throttler), got %d", numThrottlers, finalTotal)
	}

	t.Logf("Total calls across %d throttlers: %d", numThrottlers, finalTotal)
}

// TestRapidCreateAndDestroy tests creating and destroying throttlers rapidly
func TestRapidCreateAndDestroy(t *testing.T) {
	numIterations := 100
	var wg sync.WaitGroup

	for i := 0; i < numIterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			th := New(10 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			done := make(chan bool)
			go func() {
				th.Run(ctx, func() {
					// Do minimal work
				})
				done <- true
			}()

			// Trigger a few times
			th.Trigger()
			th.Trigger()
			th.Trigger()

			<-done
		}(i)
	}

	wg.Wait()
	t.Logf("Successfully created and destroyed %d throttlers concurrently", numIterations)
}

// TestContextCancellationRace tests context cancellation under high concurrency
func TestContextCancellationRace(t *testing.T) {
	th := New(25 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	// Start the throttler
	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Start many goroutines triggering
	numGoroutines := 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
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

	// Cancel after a short time
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for everything to finish
	wg.Wait()

	// Give a moment for the Run function to exit
	select {
	case <-done:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Run() did not exit promptly after context cancellation")
	}

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	t.Logf("Function called %d times before cancellation", finalCount)
}

// TestChannelBufferBehavior tests the channel buffer behavior under high load
func TestChannelBufferBehavior(t *testing.T) {
	th := New(100 * time.Millisecond)

	// Test that the channel buffer works correctly
	// Should accept exactly one value, then subsequent sends should not block

	// First trigger should succeed
	th.Trigger()

	// Subsequent triggers should not block (they'll be dropped)
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(id int) {
			th.Trigger() // Should never block
			done <- true
		}(i)
	}

	// All goroutines should complete quickly
	for i := 0; i < 100; i++ {
		select {
		case <-done:
			// Expected
		case <-time.After(10 * time.Millisecond):
			t.Errorf("Trigger %d took too long - likely blocked", i)
		}
	}
}

// TestMemoryStress tests the throttler under memory stress conditions
func TestMemoryStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stress test in short mode")
	}

	th := New(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Create some temporary memory pressure
		data := make([]byte, 1024)
		_ = data
	}

	// Start the throttler
	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Create memory pressure while triggering
	numGoroutines := 200
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Create some memory allocations
			for j := 0; j < 100; j++ {
				data := make([]byte, 1024)
				_ = data
				th.Trigger()

				if j%10 == 0 {
					runtime.GC() // Force garbage collection occasionally
				}
			}
		}()
	}

	wg.Wait()
	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	if finalCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCount)
	}

	t.Logf("Function called %d times under memory stress", finalCount)
}
