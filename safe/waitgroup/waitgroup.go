package waitgroup

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// WaitGroup is a sync.WaitGroup with checked counter updates. The zero value
// is ready to use. It must not be copied after first use.
//
// The same rule as for sync.WaitGroup applies: calls with a positive delta
// that start when the counter is zero must happen before Wait.
type WaitGroup struct {
	wg     sync.WaitGroup
	count  atomic.Int64
	panics bool
}

// Option configures a [WaitGroup].
type Option func(*WaitGroup)

// WithPanicOnMisuse makes Add and Done panic instead of returning an error
// when the counter would become negative.
func WithPanicOnMisuse() Option {
	return func(wg *WaitGroup) { wg.panics = true }
}

// NewWaitGroup returns a configured WaitGroup.
func NewWaitGroup(opts ...Option) *WaitGroup {
	wg := &WaitGroup{}
	for _, opt := range opts {
		opt(wg)
	}
	return wg
}

// Add adds delta to the counter. A delta that would make the counter
// negative (or overflow int32, the limit of sync.WaitGroup) is rejected and
// the counter is left unchanged.
func (wg *WaitGroup) Add(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		// Invariant: inner counter >= public counter. Increments hit the
		// inner group first, decrements hit the public counter first, so
		// the inner group can never go negative and is released only when
		// the public counter is zero as well.
		if cur := wg.count.Load(); cur+int64(delta) > math.MaxInt32 {
			return wg.misuse(fmt.Errorf("waitgroup: counter overflow (%d + %d)", cur, delta))
		}
		wg.wg.Add(delta)
		wg.count.Add(int64(delta))
		return nil
	}
	for {
		cur := wg.count.Load()
		next := cur + int64(delta)
		if next < 0 {
			return wg.misuse(fmt.Errorf("waitgroup: counter would go negative (%d %d)", cur, delta))
		}
		if wg.count.CompareAndSwap(cur, next) {
			break
		}
	}
	wg.wg.Add(delta)
	return nil
}

// Done decrements the counter by one; see [WaitGroup.Add].
func (wg *WaitGroup) Done() error { return wg.Add(-1) }

func (wg *WaitGroup) misuse(err error) error {
	if wg.panics {
		panic(err)
	}
	return err
}

// Go runs fn in a new goroutine tracked by the group, like
// sync.WaitGroup.Go. A panic in fn is not recovered; wrap fn with
// [github.com/jwm1rr0rb10/go-core/safe.Func] if needed.
func (wg *WaitGroup) Go(fn func()) {
	_ = wg.Add(1)
	go func() {
		defer func() { _ = wg.Done() }()
		fn()
	}()
}

// Wait blocks until the counter is zero.
func (wg *WaitGroup) Wait() { wg.wg.Wait() }

// WaitContext waits until the counter is zero or ctx is done, returning
// ctx.Err() in the latter case. On cancellation a helper goroutine keeps
// waiting in the background until the group finishes.
func (wg *WaitGroup) WaitContext(ctx context.Context) error {
	if wg.count.Load() == 0 {
		return nil
	}
	done := make(chan struct{})
	go func() {
		wg.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitTimeout waits up to d and reports whether the counter reached zero.
func (wg *WaitGroup) WaitTimeout(d time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return wg.WaitContext(ctx) == nil
}

// Count returns the current counter value.
func (wg *WaitGroup) Count() int { return int(wg.count.Load()) }
