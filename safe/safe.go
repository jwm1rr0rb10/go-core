package safe

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// PanicError is a recovered panic. Retrieve it with errors.As.
type PanicError struct {
	// Value is the value passed to panic.
	Value any
	// Stack is the goroutine stack trace captured during recovery; it
	// includes the frame that panicked.
	Stack []byte
}

// NewPanicError wraps a recovered value and captures the current stack. Call
// it from the deferred function that called recover.
func NewPanicError(v any) *PanicError {
	return &PanicError{Value: v, Stack: debug.Stack()}
}

// Error returns "panic: <value>". The stack is not part of the message; log
// [PanicError.Stack] separately.
func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// Unwrap returns the panic value when it is an error, so errors.Is works for
// panic(err).
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// RecoverFunc observes a recovered panic, e.g. to log it or bump a metric.
// It must not panic.
type RecoverFunc func(p *PanicError)

// DefaultRecover logs the panic and its stack at error level with
// slog.Default().
func DefaultRecover(p *PanicError) {
	slog.Default().Error("recovered from panic", "panic", p.Value, "stack", string(p.Stack))
}

// IgnoreRecover is a [RecoverFunc] that does nothing; use it when the
// returned error is handled anyway.
func IgnoreRecover(*PanicError) {}

func handler(fn RecoverFunc) RecoverFunc {
	if fn == nil {
		return DefaultRecover
	}
	return fn
}

// Recover converts a panic into *errp. It must be deferred directly:
//
//	func work() (err error) {
//		defer safe.Recover(&err, nil)
//		...
//	}
//
// A nil recoverFn means [DefaultRecover].
func Recover(errp *error, recoverFn RecoverFunc) {
	if r := recover(); r != nil {
		p := NewPanicError(r)
		handler(recoverFn)(p)
		if errp != nil {
			*errp = p
		}
	}
}

// Call runs fn and returns its error, or a [*PanicError] if it panicked.
func Call(fn func() error, recoverFn RecoverFunc) (err error) {
	defer Recover(&err, recoverFn)
	return fn()
}

// CallCtx is [Call] for context-aware functions.
func CallCtx(ctx context.Context, fn func(context.Context) error, recoverFn RecoverFunc) (err error) {
	defer Recover(&err, recoverFn)
	return fn(ctx)
}

// Go runs fn in a new goroutine. The returned channel (buffer 1) receives
// fn's error or a [*PanicError] and is then closed; on success it is closed
// without a value, so a receive yields nil. Receiving is optional.
func Go(ctx context.Context, fn func(context.Context) error, recoverFn RecoverFunc) <-chan error {
	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		if err := CallCtx(ctx, fn, recoverFn); err != nil {
			errc <- err
		}
	}()
	return errc
}

// Func wraps fn so that calling the result never panics.
func Func(fn func() error, recoverFn RecoverFunc) func() error {
	return func() error { return Call(fn, recoverFn) }
}

// CtxFunc wraps a context-aware fn so that calling the result never panics.
func CtxFunc(fn func(context.Context) error, recoverFn RecoverFunc) func(context.Context) error {
	return func(ctx context.Context) error { return CallCtx(ctx, fn, recoverFn) }
}

// SafeGo is the former name of [Go].
//
// Deprecated: use [Go].
func SafeGo(ctx context.Context, fn func(context.Context) error, recoverFn RecoverFunc) <-chan error {
	return Go(ctx, fn, recoverFn)
}

// SafeFunc is the former name of [Func].
//
// Deprecated: use [Func].
func SafeFunc(fn func() error, recoverFn RecoverFunc) func() error { return Func(fn, recoverFn) }

// SafeCtxFunc is the former name of [CtxFunc].
//
// Deprecated: use [CtxFunc].
func SafeCtxFunc(fn func(context.Context) error, recoverFn RecoverFunc) func(context.Context) error {
	return CtxFunc(fn, recoverFn)
}
