# Throttler

[![Go Reference](https://pkg.go.dev/badge/github.com/blackorder/throttler.svg)](https://pkg.go.dev/github.com/blackorder/throttler)
[![Go Report Card](https://goreportcard.com/badge/github.com/blackorder/throttler)](https://goreportcard.com/report/github.com/blackorder/throttler)

**Package:** `github.com/blackorder/throttler`

A simple and efficient Go library that provides time-based throttling functionality. It limits the frequency of function execution by ensuring that a function is called at most once per specified duration, while intelligently collapsing multiple triggers within the same time window into a single execution.

## Features

* **Time-based throttling** - Execute functions at most once per specified duration
* **Trigger collapsing** - Multiple rapid triggers are automatically collapsed into single executions
* **Non-blocking triggers** - Trigger calls never block, excess triggers are simply ignored
* **Context-aware** - Proper context cancellation support for clean shutdowns
* **Goroutine-safe** - Safe for concurrent use from multiple goroutines
* **High performance** - Minimal overhead and memory allocation
* **Zero dependencies** - Only uses Go standard library

## Use Cases

This throttler is particularly useful for:

- **File system watchers** that trigger on multiple rapid file changes
- **UI updates** that should be batched for better performance 
- **API calls** that need rate limiting
- **Database writes** that can be coalesced
- **Search suggestions** that should debounce user input
- **Auto-save functionality** that shouldn't save on every keystroke
- **Event processing** where rapid events should be batched

## Installation

```bash
go get github.com/blackorder/throttler
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/blackorder/throttler"
)

func main() {
    // Create a throttler that allows execution at most once per 500ms
    th := throttler.New(500 * time.Millisecond)
    ctx := context.Background()

    // Start the throttler in a goroutine
    go th.Run(ctx, func() {
        fmt.Println("Throttled function executed!")
    })

    // Multiple rapid triggers will be collapsed into a single execution
    th.Trigger()
    th.Trigger() // This will be ignored if too close to the first
    th.Trigger() // This will also be ignored

    time.Sleep(1 * time.Second)
}
```

## Detailed Usage

### Basic Throttling

```go
th := throttler.New(200 * time.Millisecond)
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go th.Run(ctx, func() {
    fmt.Println("This runs at most once every 200ms")
})

// These triggers will be collapsed
for i := 0; i < 10; i++ {
    th.Trigger()
    time.Sleep(10 * time.Millisecond) // Much faster than throttle duration
}
```

### File System Watcher Example

```go
func watchFiles() {
    th := throttler.New(1 * time.Second)
    ctx := context.Background()

    // Start the rebuild process (throttled)
    go th.Run(ctx, func() {
        fmt.Println("Rebuilding project...")
        // Your rebuild logic here
    })

    // File watcher (pseudo-code)
    watcher.OnChange(func(filename string) {
        fmt.Printf("File changed: %s\n", filename)
        th.Trigger() // Will rebuild at most once per second
    })
}
```

### Auto-save Example

```go
func setupAutoSave(document *Document) {
    th := throttler.New(2 * time.Second)
    ctx := context.Background()

    go th.Run(ctx, func() {
        fmt.Println("Auto-saving document...")
        document.Save()
    })

    // Trigger save on every document change
    document.OnChange(func() {
        th.Trigger() // Will save at most once every 2 seconds
    })
}
```

### Graceful Shutdown

```go
func main() {
    th := throttler.New(100 * time.Millisecond)
    ctx, cancel := context.WithCancel(context.Background())

    go th.Run(ctx, func() {
        fmt.Println("Processing batch...")
    })

    // Handle shutdown gracefully
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        fmt.Println("Shutting down...")
        cancel() // This will cause th.Run() to return
    }()

    // Your main application logic
    for {
        th.Trigger()
        time.Sleep(50 * time.Millisecond)
    }
}
```

## API Reference

### Types

#### `type Throttler struct`

The main throttler type. Use `New()` to create a properly initialized instance.

### Functions

#### `func New(duration time.Duration) *Throttler`

Creates a new throttler with the specified minimum duration between function executions.

**Parameters:**
- `duration`: The minimum time interval between function executions

**Returns:** A properly initialized `*Throttler`

#### `func (t *Throttler) Trigger()`

Signals that the throttled function should be considered for execution. Multiple triggers within the throttling window are collapsed into a single execution.

- Non-blocking operation
- Safe for concurrent use
- Excess triggers are ignored (not queued)

#### `func (t *Throttler) Run(ctx context.Context, fn func())`

Starts the throttler's main loop. This method blocks until the context is cancelled.

**Parameters:**
- `ctx`: Context for cancellation control
- `fn`: The function to execute in a throttled manner

**Behavior:**
- Executes `fn` at most once per configured duration
- Collapses multiple triggers into single executions
- Returns when context is cancelled
- Should typically be run in its own goroutine

## How It Works

The throttler uses a simple but effective algorithm:

1. **Trigger Phase**: When `Trigger()` is called, it sends a signal to an internal channel (non-blocking)
2. **Timer Phase**: On the first trigger, a timer is started for the configured duration
3. **Execution Phase**: When the timer expires, if there were any triggers during the wait period, the function is executed
4. **Reset Phase**: After execution, the timer is reset and the process repeats

This approach ensures that:
- Multiple rapid triggers are collapsed into single executions
- The function is never called more frequently than the specified duration
- There's no unbounded queuing of triggers
- Memory usage remains constant regardless of trigger frequency

## Performance

The throttler is designed for high performance:

- **Trigger operations**: ~1.8 ns/op (no allocations)
- **Memory usage**: Constant, regardless of trigger frequency  
- **Goroutine overhead**: Single additional goroutine for the throttler loop

Benchmark results:
```
BenchmarkTrigger-20                  	689142022	         1.795 ns/op
BenchmarkThrottlerWithHighLoad-20    	521185474	         2.398 ns/op
```

## Testing

The package includes comprehensive tests covering:

- Basic functionality and edge cases
- Timing behavior and proper throttling
- Context cancellation
- Concurrent usage and race conditions
- Performance benchmarks

Run tests:
```bash
go test -v                    # Basic tests
go test -race -v             # Race condition tests
go test -bench=.             # Performance benchmarks
```

### Race Condition Testing

The package has been extensively tested for race conditions with:
- 1000+ concurrent goroutines
- 500,000+ triggers under extreme load
- Context cancellation under high concurrency
- Timer edge cases and rapid lifecycle testing

See [RACE_TESTING.md](RACE_TESTING.md) for detailed race testing results.

## Thread Safety

The throttler is fully thread-safe:

- `Trigger()` can be called safely from multiple goroutines
- `Run()` should only be called once per throttler instance
- No external synchronization is required

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Related Projects

If you need different throttling behavior, consider:

- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) - Token bucket rate limiter
- [go-rate](https://github.com/beefsack/go-rate) - Simple rate limiter
- [ratelimit](https://github.com/uber-go/ratelimit) - Uber's rate limiter
