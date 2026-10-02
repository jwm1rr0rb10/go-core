package errorgroup

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jwm1rr0rb10/go-core/safe"
)

// Group is a collection of goroutines working on subtasks of the same task.
// The zero value is usable: tasks receive context.Background, no context is
// cancelled, and Wait returns the first error. A Group must not be copied
// after first use or reused after Wait.
type Group struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	wg     sync.WaitGroup
	sem    chan struct{}

	recover         safe.RecoverFunc
	collectAll      bool
	continueOnError bool

	mu   sync.Mutex
	errs []error
}

// SafeGroup is the former name of [Group].
//
// Deprecated: use [Group].
type SafeGroup = Group

// RecoverFunc observes a recovered panic.
//
// Deprecated: use [safe.RecoverFunc]; this alias exists for compatibility.
type RecoverFunc = safe.RecoverFunc

// DefaultRecover logs a recovered panic with slog.Default.
//
// Deprecated: use [safe.DefaultRecover].
func DefaultRecover(p *safe.PanicError) { safe.DefaultRecover(p) }

// Option configures a [Group].
type Option func(*Group)

// WithRecover sets the panic observer. nil means [safe.DefaultRecover]; use
// [safe.IgnoreRecover] to stay silent.
func WithRecover(fn safe.RecoverFunc) Option {
	return func(g *Group) { g.recover = fn }
}

// WithCollectAll makes [Group.Wait] return all task errors joined with
// errors.Join instead of only the first one.
func WithCollectAll() Option {
	return func(g *Group) { g.collectAll = true }
}

// WithContinueOnError keeps the group context alive when a task fails, so
// the remaining tasks are not asked to stop. Usually combined with
// [WithCollectAll].
func WithContinueOnError() Option {
	return func(g *Group) { g.continueOnError = true }
}

// WithLimit is [Group.SetLimit] as an option.
func WithLimit(n int) Option {
	return func(g *Group) { g.SetLimit(n) }
}

// New returns a Group whose tasks receive context.Background.
func New(opts ...Option) *Group {
	g := &Group{}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// WithContext returns a Group and a context derived from ctx. The context is
// cancelled when a task fails (unless [WithContinueOnError] is set) and when
// Wait returns; context.Cause reports the first error.
func WithContext(ctx context.Context, opts ...Option) (*Group, context.Context) {
	g := New(opts...)
	g.ctx, g.cancel = context.WithCancelCause(ctx)
	return g, g.ctx
}

// SetLimit limits the number of active goroutines to n; n < 0 removes the
// limit. It must not be called while tasks are running.
func (g *Group) SetLimit(n int) {
	if n < 0 {
		g.sem = nil
		return
	}
	if len(g.sem) != 0 {
		panic(fmt.Errorf("errorgroup: modify limit while %v goroutines in the group are still active", len(g.sem)))
	}
	g.sem = make(chan struct{}, n)
}

// Go runs fn in a new goroutine, blocking first while the limit is reached.
// fn receives the group context. A panic in fn is recovered and recorded as
// a *safe.PanicError.
func (g *Group) Go(fn func(ctx context.Context) error) {
	if g.sem != nil {
		g.sem <- struct{}{}
	}
	g.start(fn)
}

// TryGo is like Go but returns false without starting fn when the limit is
// reached.
func (g *Group) TryGo(fn func(ctx context.Context) error) bool {
	if g.sem != nil {
		select {
		case g.sem <- struct{}{}:
		default:
			return false
		}
	}
	g.start(fn)
	return true
}

func (g *Group) start(fn func(ctx context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer func() {
			if g.sem != nil {
				<-g.sem
			}
			g.wg.Done()
		}()
		ctx := g.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := safe.CallCtx(ctx, fn, g.recover); err != nil {
			g.record(err)
		}
	}()
}

func (g *Group) record(err error) {
	g.mu.Lock()
	first := len(g.errs) == 0
	g.errs = append(g.errs, err)
	g.mu.Unlock()
	if first && g.cancel != nil && !g.continueOnError {
		g.cancel(err)
	}
}

// Wait blocks until all tasks have returned, cancels the group context and
// returns the first error, or all errors joined with [WithCollectAll]. It
// returns nil if every task succeeded.
func (g *Group) Wait() error {
	g.wg.Wait()
	if g.cancel != nil {
		g.cancel(context.Canceled)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case len(g.errs) == 0:
		return nil
	case g.collectAll:
		return errors.Join(g.errs...)
	default:
		return g.errs[0]
	}
}

// Errors returns a copy of all errors recorded so far, in completion order.
func (g *Group) Errors() []error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]error(nil), g.errs...)
}
