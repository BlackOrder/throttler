package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blackorder/throttler"
)

func main() {
	// Create a throttler that allows execution at most once per 500ms
	th := throttler.New(500 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Counter to track executions
	var execCount int

	// Start the throttler in a goroutine
	go th.Run(ctx, func() {
		execCount++
		fmt.Printf("Throttled function executed! (count: %d)\n", execCount)
	})

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down gracefully...")
		cancel()
		os.Exit(0)
	}()

	fmt.Println("Throttler example running. Press Ctrl+C to exit.")
	fmt.Println("Triggering rapidly - watch how calls are throttled to max once per 500ms:")

	// Simulate rapid triggers
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	triggerCount := 0
	for {
		select {
		case <-ticker.C:
			triggerCount++
			th.Trigger()
			fmt.Printf("Trigger %d sent\n", triggerCount)
		case <-ctx.Done():
			return
		}
	}
}
