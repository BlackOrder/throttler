package throttler

import (
	"context"
	"time"
)

type Throttler struct {
	C        chan struct{}
	Duration time.Duration
}

func New(t time.Duration) *Throttler {
	return &Throttler{
		C:        make(chan struct{}, 1),
		Duration: t,
	}
}

// Trigger tells the Throttler “something changed”.
// Multiple triggers within the window collapse into one.
func (t *Throttler) Trigger() {
	select {
	case t.C <- struct{}{}:
	default:
	}
}

// Run will call fn() at most once per Duration,
// and at least once every Duration if Trigger() is being called.
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
