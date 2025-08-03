package throttler

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	duration := 100 * time.Millisecond
	th := New(duration)

	if th == nil {
		t.Fatal("New() returned nil")
	}

	if th.Duration != duration {
		t.Errorf("Expected duration %v, got %v", duration, th.Duration)
	}

	if th.C == nil {
		t.Fatal("Channel C is nil")
	}

	// Test that channel has buffer size of 1
	th.C <- struct{}{}
	select {
	case th.C <- struct{}{}:
		t.Error("Channel should not accept second value without blocking")
	default:
		// Expected behavior
	}
}

func TestTrigger(t *testing.T) {
	th := New(100 * time.Millisecond)

	// First trigger should succeed
	th.Trigger()

	// Second trigger should not block (non-blocking send)
	done := make(chan bool, 1)
	go func() {
		th.Trigger()
		done <- true
	}()

	select {
	case <-done:
		// Expected - should not block
	case <-time.After(10 * time.Millisecond):
		t.Error("Trigger() blocked when it should be non-blocking")
	}
}

func TestThrottlerBasicOperation(t *testing.T) {
	th := New(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
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

	// Trigger multiple times quickly
	th.Trigger()
	th.Trigger()
	th.Trigger()

	// Wait for context to timeout
	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have been called at least once
	if finalCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCount)
	}

	// Should not have been called for every trigger due to throttling
	if finalCount > 2 {
		t.Errorf("Expected at most 2 calls due to throttling, got %d", finalCount)
	}
}

func TestThrottlerContextCancellation(t *testing.T) {
	th := New(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	var callCount int
	fn := func() {
		callCount++
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Cancel immediately
	cancel()

	// Should exit quickly
	select {
	case <-done:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("Run() did not exit promptly after context cancellation")
	}
}

func TestThrottlerTimingBehavior(t *testing.T) {
	duration := 100 * time.Millisecond
	th := New(duration)
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	var calls []time.Time
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		calls = append(calls, time.Now())
		mu.Unlock()
	}

	start := time.Now()

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Trigger immediately
	th.Trigger()

	// Wait a bit, then trigger again
	time.Sleep(50 * time.Millisecond)
	th.Trigger()

	// Wait for completion
	<-done

	mu.Lock()
	callTimes := make([]time.Time, len(calls))
	copy(callTimes, calls)
	mu.Unlock()

	if len(callTimes) < 1 {
		t.Fatal("Expected at least one function call")
	}

	// First call should happen after the initial duration
	firstCallDelay := callTimes[0].Sub(start)
	if firstCallDelay < duration-10*time.Millisecond {
		t.Errorf("First call happened too early: %v, expected at least %v", firstCallDelay, duration)
	}

	// If there were multiple calls, they should be properly spaced
	if len(callTimes) > 1 {
		for i := 1; i < len(callTimes); i++ {
			interval := callTimes[i].Sub(callTimes[i-1])
			if interval < duration-10*time.Millisecond {
				t.Errorf("Calls %d and %d were too close: %v, expected at least %v", i-1, i, interval, duration)
			}
		}
	}
}

func TestThrottlerRepeatedTriggers(t *testing.T) {
	th := New(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Trigger multiple times with some spacing
	for i := 0; i < 5; i++ {
		th.Trigger()
		time.Sleep(20 * time.Millisecond)
	}

	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have consolidated multiple triggers
	if finalCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", finalCount)
	}
	if finalCount > 3 {
		t.Errorf("Expected at most 3 calls due to throttling, got %d", finalCount)
	}
}

func TestThrottlerNoPendingAfterTimer(t *testing.T) {
	th := New(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var callCount int
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	done := make(chan bool)
	go func() {
		th.Run(ctx, fn)
		done <- true
	}()

	// Single trigger
	th.Trigger()

	// Wait for the timer to fire and reset
	time.Sleep(100 * time.Millisecond)

	// Another trigger after reset
	th.Trigger()

	<-done

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have at least 2 calls (one for each trigger period)
	if finalCount < 2 {
		t.Errorf("Expected at least 2 calls, got %d", finalCount)
	}
}

// Benchmark tests
func BenchmarkTrigger(b *testing.B) {
	th := New(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th.Trigger()
	}
}

func BenchmarkThrottlerWithHighLoad(b *testing.B) {
	th := New(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go th.Run(ctx, func() {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th.Trigger()
	}
}
