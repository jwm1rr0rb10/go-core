package repeat

import (
	"errors"
	"strconv"
	"time"
)

var (
	// ErrInvalidConfig is returned by [New], [Exec] and [Do] for an invalid
	// option combination.
	ErrInvalidConfig = errors.New("repeat: invalid config")

	// ErrMaxElapsed is the cause reported when the next retry would exceed
	// the [WithMaxElapsed] budget.
	ErrMaxElapsed = errors.New("repeat: max elapsed time exceeded")
)

type configError string

func (e configError) Error() string { return ErrInvalidConfig.Error() + ": " + string(e) }
func (e configError) Unwrap() error { return ErrInvalidConfig }

func invalid(msg string) error { return configError(msg) }

// Error is returned when retrying gave up because of the attempt or time
// budget or the context. It unwraps to both the last operation error and,
// when set, the cause, so errors.Is(err, context.DeadlineExceeded) and
// errors.Is(err, io.EOF) can both hold for the same value.
type Error struct {
	Attempts int   // attempts made, including the first one
	Err      error // error of the last attempt
	Cause    error // why retrying stopped early: ctx error or ErrMaxElapsed; nil if attempts ran out
}

// Error implements error.
func (e *Error) Error() string {
	msg := "repeat: gave up after " + strconv.Itoa(e.Attempts) + " attempt"
	if e.Attempts != 1 {
		msg += "s"
	}
	if e.Cause != nil {
		msg += " (" + e.Cause.Error() + ")"
	}
	return msg + ": " + e.Err.Error()
}

// Unwrap returns the last operation error and the cause.
func (e *Error) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Err}
	}
	return []error{e.Err, e.Cause}
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as non-retryable: the retrier returns it immediately,
// unwrapped. Permanent(nil) returns nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with [Permanent].
func IsPermanent(err error) bool {
	_, ok := errors.AsType[*permanentError](err)
	return ok
}

type retryAfterError struct {
	err error
	d   time.Duration
}

func (e *retryAfterError) Error() string             { return e.err.Error() }
func (e *retryAfterError) Unwrap() error             { return e.err }
func (e *retryAfterError) RetryAfter() time.Duration { return e.d }

// RetryAfter wraps err with a minimum delay before the next attempt, e.g.
// from a server hint. The retrier waits max(backoff, d). Any error in the
// chain that has a RetryAfter() time.Duration method is honored the same way.
// RetryAfter(nil, d) returns nil.
func RetryAfter(err error, d time.Duration) error {
	if err == nil {
		return nil
	}
	return &retryAfterError{err: err, d: d}
}

type retryAfterer interface {
	error
	RetryAfter() time.Duration
}

func retryAfterHint(err error) (time.Duration, bool) {
	if ra, ok := errors.AsType[retryAfterer](err); ok {
		return ra.RetryAfter(), true
	}
	return 0, false
}
