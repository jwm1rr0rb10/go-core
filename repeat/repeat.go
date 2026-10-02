package repeat

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// Operation is a unit of work to retry. attempt is 0 for the first call and
// grows by one on every retry.
type Operation func(ctx context.Context, attempt int) error

// Jitter selects how a computed backoff delay is randomized.
type Jitter uint8

const (
	// JitterFull waits a uniformly random duration in [0, d]. It spreads
	// retries from many clients best and is the default.
	JitterFull Jitter = iota
	// JitterNone waits exactly d.
	JitterNone
	// JitterEqual waits d/2 plus a random duration in [0, d/2].
	JitterEqual
	// JitterDecorrelated waits a random duration in
	// [base, min(max, 3·previous delay)], as described in the AWS
	// "Exponential Backoff and Jitter" article.
	JitterDecorrelated
)

// Unlimited disables the attempt limit when passed to [WithMaxAttempts].
// Combine it with [WithMaxElapsed] or a context deadline.
const Unlimited = -1

// Defaults used when the corresponding option is not set.
const (
	DefaultMaxAttempts = 4
	DefaultBaseDelay   = 100 * time.Millisecond
	DefaultMaxDelay    = 10 * time.Second
)

// RetryEvent describes a failed attempt that is about to be retried. It is
// passed to the [WithOnRetry] hook.
type RetryEvent struct {
	Attempt int           // 0-based number of the attempt that failed
	Err     error         // error returned by that attempt
	Delay   time.Duration // wait before the next attempt
}

// Option configures a [Retrier].
type Option func(*config)

type config struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	maxElapsed  time.Duration
	jitter      Jitter
	retryIf     func(error) bool
	policy      func(attempt int, err error) bool
	onRetry     func(RetryEvent)
}

// WithMaxAttempts sets the total number of attempts, including the first
// one: 1 disables retries, [Unlimited] removes the limit. Values of 0 or
// below -1 make [New] and [Exec] fail with [ErrInvalidConfig].
func WithMaxAttempts(n int) Option {
	return func(c *config) { c.maxAttempts = n }
}

// WithBaseDelay sets the delay before the first retry; the n-th retry waits
// base·2ⁿ before jitter, capped by [WithMaxDelay].
func WithBaseDelay(d time.Duration) Option {
	return func(c *config) { c.baseDelay = d }
}

// WithMaxDelay caps a single backoff delay.
func WithMaxDelay(d time.Duration) Option {
	return func(c *config) { c.maxDelay = d }
}

// WithBackoff sets base and max delay in one call.
func WithBackoff(base, max time.Duration) Option {
	return func(c *config) { c.baseDelay, c.maxDelay = base, max }
}

// WithConstantDelay waits exactly d between attempts.
func WithConstantDelay(d time.Duration) Option {
	return func(c *config) { c.baseDelay, c.maxDelay, c.jitter = d, d, JitterNone }
}

// WithJitter selects the jitter strategy. The default is [JitterFull].
func WithJitter(j Jitter) Option {
	return func(c *config) { c.jitter = j }
}

// WithMaxElapsed bounds the total time spent retrying, measured from the
// start of the first attempt. A retry whose delay would cross the budget is
// not started and the call fails with an error matching [ErrMaxElapsed].
// Zero (the default) means no limit.
func WithMaxElapsed(d time.Duration) Option {
	return func(c *config) { c.maxElapsed = d }
}

// WithRetryIf retries only errors for which fn returns true. Errors wrapped
// with [Permanent] are never retried regardless of fn.
func WithRetryIf(fn func(err error) bool) Option {
	return func(c *config) { c.retryIf = fn }
}

// WithRetryPolicy is like [WithRetryIf] but also receives the 0-based number
// of the attempt that failed. It takes precedence over [WithRetryIf].
func WithRetryPolicy(fn func(attempt int, err error) bool) Option {
	return func(c *config) { c.policy = fn }
}

// WithOnRetry registers a hook called after each failed attempt that will be
// retried, before waiting. Use it for logging and metrics. It runs on the
// retrying goroutine and must not block.
func WithOnRetry(fn func(RetryEvent)) Option {
	return func(c *config) { c.onRetry = fn }
}

// Retrier executes operations with a fixed retry policy. It is immutable and
// safe for concurrent use; build it once with [New] and reuse it.
type Retrier struct {
	cfg config
}

// New validates opts and returns a reusable [Retrier].
func New(opts ...Option) (*Retrier, error) {
	r := &Retrier{cfg: config{
		maxAttempts: DefaultMaxAttempts,
		baseDelay:   DefaultBaseDelay,
		maxDelay:    DefaultMaxDelay,
		jitter:      JitterFull,
	}}
	for _, opt := range opts {
		if opt != nil {
			opt(&r.cfg)
		}
	}
	if err := r.cfg.validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (c *config) validate() error {
	switch {
	case c.maxAttempts == 0 || c.maxAttempts < Unlimited:
		return invalid("max attempts must be positive or Unlimited")
	case c.baseDelay < 0 || c.maxDelay < 0 || c.maxElapsed < 0:
		return invalid("durations must not be negative")
	case c.maxDelay < c.baseDelay:
		return invalid("max delay is less than base delay")
	case c.jitter > JitterDecorrelated:
		return invalid("unknown jitter strategy")
	}
	return nil
}

// Exec runs op until it succeeds or the policy gives up. See the package
// documentation for the stop conditions and the shape of the returned error.
// The config is validated on every call; prefer [New] + [Retrier.Exec] on hot
// paths.
func Exec(ctx context.Context, op Operation, opts ...Option) error {
	r, err := New(opts...)
	if err != nil {
		return err
	}
	return r.Exec(ctx, op)
}

// Do is the generic form of [Exec] for operations that return a value. On
// failure it returns the zero value of T.
func Do[T any](ctx context.Context, fn func(ctx context.Context) (T, error), opts ...Option) (T, error) {
	r, err := New(opts...)
	if err != nil {
		var zero T
		return zero, err
	}
	return DoWith(ctx, r, fn)
}

// DoWith is [Do] with a prebuilt [Retrier].
func DoWith[T any](ctx context.Context, r *Retrier, fn func(ctx context.Context) (T, error)) (T, error) {
	var res T
	err := r.Exec(ctx, func(ctx context.Context, _ int) error {
		v, err := fn(ctx)
		if err == nil {
			res = v
		}
		return err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return res, nil
}

// Exec runs op with the retrier's policy.
func (r *Retrier) Exec(ctx context.Context, op Operation) error {
	return r.exec(ctx, op, nil)
}

// exec is Exec with an internal hook invoked once a retry has been decided,
// right before waiting (used by Transport to release the previous response).
func (r *Retrier) exec(ctx context.Context, op Operation, beforeWait func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c := &r.cfg
	var (
		start  = time.Now()
		prev   = c.baseDelay
		timer  *time.Timer
		lastEr error
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for attempt := 0; ; attempt++ {
		lastEr = op(ctx, attempt)
		if lastEr == nil {
			return nil
		}
		if perm, ok := errors.AsType[*permanentError](lastEr); ok {
			if lastEr == error(perm) {
				return perm.err
			}
			return lastEr
		}
		if !c.shouldRetry(attempt, lastEr) {
			return lastEr
		}
		if c.maxAttempts != Unlimited && attempt+1 >= c.maxAttempts {
			return &Error{Attempts: attempt + 1, Err: lastEr}
		}
		if err := ctx.Err(); err != nil {
			return &Error{Attempts: attempt + 1, Err: lastEr, Cause: err}
		}

		delay := c.delay(attempt, &prev)
		if hint, ok := retryAfterHint(lastEr); ok && hint > delay {
			delay = hint
		}
		if c.maxElapsed > 0 && time.Since(start)+delay > c.maxElapsed {
			return &Error{Attempts: attempt + 1, Err: lastEr, Cause: ErrMaxElapsed}
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < delay {
			return &Error{Attempts: attempt + 1, Err: lastEr, Cause: context.DeadlineExceeded}
		}

		if c.onRetry != nil {
			c.onRetry(RetryEvent{Attempt: attempt, Err: lastEr, Delay: delay})
		}
		if beforeWait != nil {
			beforeWait()
		}
		if delay <= 0 {
			continue
		}
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			timer.Reset(delay)
		}
		select {
		case <-ctx.Done():
			return &Error{Attempts: attempt + 1, Err: lastEr, Cause: ctx.Err()}
		case <-timer.C:
		}
	}
}

func (c *config) shouldRetry(attempt int, err error) bool {
	if c.policy != nil {
		return c.policy(attempt, err)
	}
	if c.retryIf != nil {
		return c.retryIf(err)
	}
	return true
}

// delay returns the wait after the given failed attempt. prev holds the
// previous delay for decorrelated jitter.
func (c *config) delay(attempt int, prev *time.Duration) time.Duration {
	if c.jitter == JitterDecorrelated {
		hi := c.maxDelay
		if *prev <= c.maxDelay/3 { // overflow-safe min(max, 3·prev)
			hi = *prev * 3
		}
		d := c.baseDelay
		if hi > d {
			d += rand.N(hi - d + 1)
		}
		*prev = d
		return d
	}
	d := Backoff(c.baseDelay, c.maxDelay, attempt)
	switch c.jitter {
	case JitterFull:
		if d > 0 {
			d = rand.N(d + 1)
		}
	case JitterEqual:
		if half := d / 2; half > 0 {
			d = d - half + rand.N(half+1)
		}
	}
	return d
}

// Backoff returns min(limit, base·2^attempt) without overflowing. It is the
// un-jittered delay used by [Retrier].
func Backoff(base, limit time.Duration, attempt int) time.Duration {
	if base <= 0 || attempt < 0 {
		return max(0, min(base, limit))
	}
	if attempt >= 63 || base > limit>>uint(attempt) {
		return limit
	}
	return base << uint(attempt)
}
