package closer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jwm1rr0rb10/go-core/safe"
)

// Closer is a resource whose Close returns an error. It is io.Closer.
type Closer = io.Closer

// NoErrCloser is a resource whose Close returns nothing.
type NoErrCloser interface {
	Close()
}

// CloserFunc adapts a function to [Closer].
type CloserFunc func() error

// Close calls f.
func (f CloserFunc) Close() error { return f() }

// NoErrCloserFunc adapts a function to [NoErrCloser].
type NoErrCloserFunc func()

// Close calls f.
func (f NoErrCloserFunc) Close() { f() }

// DefaultSignals are the signals the CloseOnSignal functions wait for when
// none are given.
var DefaultSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Option configures a [LIFOCloser].
type Option func(*LIFOCloser)

// WithTimeout bounds the whole Close call. Zero means no limit (or the
// limit of the context passed to [LIFOCloser.CloseContext]).
func WithTimeout(d time.Duration) Option {
	return func(lc *LIFOCloser) { lc.timeout = d }
}

// WithCloserTimeout bounds each individual closer. Zero means no limit.
func WithCloserTimeout(d time.Duration) Option {
	return func(lc *LIFOCloser) { lc.closerTimeout = d }
}

// WithLogger logs shutdown progress and failures to l. By default nothing
// is logged.
func WithLogger(l *slog.Logger) Option {
	return func(lc *LIFOCloser) {
		if l != nil {
			lc.logger = l
		}
	}
}

type entry struct {
	name string
	fn   func(context.Context) error
}

// LIFOCloser closes registered resources in Last-In-First-Out order. It is
// safe for concurrent use. The zero value is usable and has no timeouts.
type LIFOCloser struct {
	timeout       time.Duration
	closerTimeout time.Duration
	logger        *slog.Logger

	mu      sync.Mutex
	entries []entry
	closed  bool

	once sync.Once
	err  error
}

var discard = slog.New(slog.DiscardHandler)

// NewLIFOCloser returns a configured LIFOCloser.
func NewLIFOCloser(opts ...Option) *LIFOCloser {
	lc := &LIFOCloser{}
	for _, opt := range opts {
		opt(lc)
	}
	return lc
}

func (lc *LIFOCloser) log() *slog.Logger {
	if lc.logger == nil {
		return discard
	}
	return lc.logger
}

// Add registers closers. Each is named after its dynamic type in error
// messages; use [LIFOCloser.AddNamed] for a better name.
func (lc *LIFOCloser) Add(closers ...Closer) {
	for _, c := range closers {
		if c != nil {
			lc.AddNamed(fmt.Sprintf("%T", c), c)
		}
	}
}

// AddNamed registers c under name.
func (lc *LIFOCloser) AddNamed(name string, c Closer) {
	lc.add(entry{name: name, fn: func(context.Context) error { return c.Close() }})
}

// AddNoErr registers closers whose Close returns nothing.
func (lc *LIFOCloser) AddNoErr(closers ...NoErrCloser) {
	for _, c := range closers {
		if c != nil {
			lc.add(entry{name: fmt.Sprintf("%T", c), fn: func(context.Context) error { c.Close(); return nil }})
		}
	}
}

// AddFunc registers a context-aware close function, such as
// (*http.Server).Shutdown or (*grpc.Server) wrappers. fn receives a context
// that expires with the per-closer or overall timeout.
func (lc *LIFOCloser) AddFunc(name string, fn func(ctx context.Context) error) {
	if fn != nil {
		lc.add(entry{name: name, fn: fn})
	}
}

// add appends e, or closes it right away if Close has already started, so a
// resource registered during shutdown is not leaked.
func (lc *LIFOCloser) add(e entry) {
	lc.mu.Lock()
	if !lc.closed {
		lc.entries = append(lc.entries, e)
		lc.mu.Unlock()
		return
	}
	lc.mu.Unlock()
	if err := lc.run(context.Background(), e); err != nil {
		lc.log().Error("closer: resource added after Close failed to close", "name", e.name, "err", err)
	}
}

// Len returns the number of resources waiting to be closed.
func (lc *LIFOCloser) Len() int {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return len(lc.entries)
}

// Close closes all resources; see [LIFOCloser.CloseContext].
func (lc *LIFOCloser) Close() error { return lc.CloseContext(context.Background()) }

// CloseContext closes all registered resources in reverse order and returns
// their errors joined. Only the first call does the work; concurrent and
// later calls wait for it and return the same error.
//
// ctx, the [WithTimeout] limit and the [WithCloserTimeout] limit bound the
// time spent waiting. A closer that does not finish in time is reported
// with the context error and keeps running in the background; the next
// closer starts immediately.
func (lc *LIFOCloser) CloseContext(ctx context.Context) error {
	lc.once.Do(func() {
		lc.mu.Lock()
		entries := lc.entries
		lc.entries, lc.closed = nil, true
		lc.mu.Unlock()

		if lc.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, lc.timeout)
			defer cancel()
		}

		log := lc.log()
		log.Info("closer: closing resources", "count", len(entries))
		var errs []error
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if err := lc.run(ctx, e); err != nil {
				log.Error("closer: close failed", "name", e.name, "err", err)
				errs = append(errs, fmt.Errorf("close %s: %w", e.name, err))
			}
		}
		lc.err = errors.Join(errs...)
		log.Info("closer: done", "failed", len(errs))
	})
	return lc.err
}

func (lc *LIFOCloser) run(ctx context.Context, e entry) error {
	if lc.closerTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, lc.closerTimeout)
		defer cancel()
	}
	log := lc.log()
	onPanic := func(p *safe.PanicError) {
		log.Error("closer: panic while closing", "name", e.name, "panic", p.Value, "stack", string(p.Stack))
	}
	if ctx.Done() == nil {
		return safe.CallCtx(ctx, e.fn, onPanic)
	}
	done := make(chan error, 1)
	go func() { done <- safe.CallCtx(ctx, e.fn, onPanic) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// The closer may have finished at the same moment; prefer its result.
		select {
		case err := <-done:
			return err
		default:
		}
		return fmt.Errorf("not finished in time: %w", ctx.Err())
	}
}

// CloseOnSignal blocks until one of signals (default [DefaultSignals])
// arrives, then closes lc and returns its error.
//
// Signal handling is restored to the default right after the first signal,
// so a second Ctrl+C terminates a hung shutdown immediately.
func CloseOnSignal(lc *LIFOCloser, signals ...os.Signal) error {
	return CloseOnSignalWithContext(context.Background(), lc, signals...)
}

// CloseOnSignalWithContext is [CloseOnSignal] that also starts shutdown when
// ctx is done. ctx only triggers shutdown; the time allowed for closing is
// set with [WithTimeout].
func CloseOnSignalWithContext(ctx context.Context, lc *LIFOCloser, signals ...os.Signal) error {
	if len(signals) == 0 {
		signals = DefaultSignals
	}
	sigCtx, stop := signal.NotifyContext(ctx, signals...)
	<-sigCtx.Done()
	stop() // next signal gets default handling: the process exits
	lc.log().Info("closer: shutdown initiated", "cause", context.Cause(sigCtx))
	return lc.Close()
}

// CloseOnSignalContext returns a function that runs
// [CloseOnSignalWithContext]; handy for errgroup-style runners.
func CloseOnSignalContext(lc *LIFOCloser, signals ...os.Signal) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return CloseOnSignalWithContext(ctx, lc, signals...)
	}
}
