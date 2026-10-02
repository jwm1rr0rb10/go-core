package clock

import (
	"context"
	"time"
)

// Clock is the source of time used by code that needs to be testable.
// Every method behaves like its counterpart in the time package.
// Implementations are safe for concurrent use.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Since returns the time elapsed since t.
	Since(t time.Time) time.Duration
	// Until returns the duration until t.
	Until(t time.Time) time.Duration
	// Sleep pauses the calling goroutine for at least d.
	// A non-positive d returns immediately.
	Sleep(d time.Duration)
	// SleepContext pauses for d or until ctx is done, whichever comes first.
	// It returns ctx.Err() if the context ended the wait, nil otherwise.
	SleepContext(ctx context.Context, d time.Duration) error
	// After waits for d and then sends the current time on the returned channel.
	After(d time.Duration) <-chan time.Time
	// NewTimer creates a Timer that sends the current time on its channel after d.
	NewTimer(d time.Duration) Timer
	// AfterFunc waits for d and then calls f in its own goroutine.
	// The returned Timer's C method returns nil.
	AfterFunc(d time.Duration, f func()) Timer
	// NewTicker returns a Ticker that ticks every d. It panics if d <= 0.
	NewTicker(d time.Duration) Ticker
}

// Timer is the interface counterpart of *time.Timer.
type Timer interface {
	// C returns the channel on which the time is delivered.
	// It is nil for timers created by AfterFunc.
	C() <-chan time.Time
	// Stop prevents the timer from firing. It reports whether the call
	// stopped the timer (false if it had already fired or been stopped).
	Stop() bool
	// Reset changes the timer to expire after d. It reports whether the
	// timer had been active.
	Reset(d time.Duration) bool
}

// Ticker is the interface counterpart of *time.Ticker.
type Ticker interface {
	// C returns the channel on which ticks are delivered.
	C() <-chan time.Time
	// Stop turns off the ticker. No more ticks are sent after Stop returns.
	Stop()
	// Reset stops the ticker and resets its period to d. It panics if d <= 0.
	Reset(d time.Duration)
}

// New returns a Clock backed by the time package.
func New() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) Since(t time.Time) time.Duration        { return time.Since(t) }
func (realClock) Until(t time.Time) time.Duration        { return time.Until(t) }
func (realClock) Sleep(d time.Duration)                  { time.Sleep(d) }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (realClock) NewTimer(d time.Duration) Timer         { return realTimer{time.NewTimer(d)} }
func (realClock) NewTicker(d time.Duration) Ticker       { return realTicker{time.NewTicker(d)} }

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return realTimer{time.AfterFunc(d, f)}
}

func (realClock) SleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	return sleepContext(ctx, t.C, t.Stop)
}

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time   { return r.t.C }
func (r realTicker) Stop()                 { r.t.Stop() }
func (r realTicker) Reset(d time.Duration) { r.t.Reset(d) }

// sleepContext waits on fired or ctx. stop, if non-nil, is called when the
// context wins so the underlying timer can be released.
func sleepContext(ctx context.Context, fired <-chan time.Time, stop func() bool) error {
	if err := ctx.Err(); err != nil {
		if stop != nil {
			stop()
		}
		return err
	}
	select {
	case <-fired:
		return nil
	case <-ctx.Done():
		if stop != nil {
			stop()
		}
		return ctx.Err()
	}
}
