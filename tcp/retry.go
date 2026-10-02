package tcp

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

// RetryPolicy controls Client retries: exponential backoff with full jitter
// (sleep = random in [0, min(MaxDelay, BaseDelay·2^attempt)]), as recommended
// for avoiding synchronized retry storms.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts including the first
	// (values < 1 mean 1).
	MaxAttempts int
	// BaseDelay is the backoff for the first retry.
	BaseDelay time.Duration
	// MaxDelay caps a single backoff.
	MaxDelay time.Duration
}

// DefaultRetryPolicy is used when no policy is configured.
var DefaultRetryPolicy = RetryPolicy{MaxAttempts: 3, BaseDelay: 50 * time.Millisecond, MaxDelay: 2 * time.Second}

func (p RetryPolicy) normalize() RetryPolicy {
	p.MaxAttempts = max(p.MaxAttempts, 1)
	p.BaseDelay = max(p.BaseDelay, 0)
	p.MaxDelay = max(p.MaxDelay, p.BaseDelay)
	return p
}

// Backoff returns the jittered delay before retry number attempt (0-based).
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if p.BaseDelay <= 0 {
		return 0
	}
	ceiling := p.MaxDelay
	if attempt < 62 {
		if d := p.BaseDelay << attempt; d > 0 && d < ceiling {
			ceiling = d
		}
	}
	return rand.N(ceiling + 1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Do runs fn and, while it fails with a retryable error ([IsRetryable]),
// reconnects and runs it again according to the retry policy. Put a whole
// request/response exchange in fn so a retry never resumes a half-finished
// exchange on a new connection. The client is connected first if needed.
//
// Retries give at-least-once semantics: a request may reach the server more
// than once, so use Do with idempotent operations.
func (c *Client) Do(ctx context.Context, fn func(ctx context.Context, c *Client) error) error {
	return c.withRetry(ctx, true, func() error { return fn(ctx, c) })
}

// WriteWithRetry writes data, reconnecting and retrying on retryable errors.
// A write that failed midway may have been partially delivered; prefer Do for
// request/response protocols.
func (c *Client) WriteWithRetry(ctx context.Context, data []byte) error {
	return c.withRetry(ctx, true, func() error { return c.Write(ctx, data) })
}

// ReadWithRetry reads one chunk, retrying on retryable errors. Read timeouts
// are retried on the same connection; connection failures trigger a reconnect.
func (c *Client) ReadWithRetry(ctx context.Context) ([]byte, error) {
	var out []byte
	err := c.withRetry(ctx, false, func() error {
		var err error
		out, err = c.Read(ctx)
		return err
	})
	return out, err
}

func (c *Client) withRetry(ctx context.Context, reconnectOnTimeout bool, fn func() error) error {
	p := c.retry
	var lastErr error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if attempt > 0 {
			c.retries.Add(1)
			if err := sleepCtx(ctx, p.Backoff(attempt-1)); err != nil {
				return errors.Join(err, lastErr)
			}
			if reconnectOnTimeout || !errors.Is(lastErr, ErrTimeout) || !c.Connected() {
				if err := c.Reconnect(ctx); err != nil {
					if !IsRetryable(err) {
						return errors.Join(err, lastErr)
					}
					lastErr = err
					c.logger.Debug("tcp: reconnect failed", "attempt", attempt, "err", err)
					continue
				}
			}
		} else if err := c.Connect(ctx); err != nil {
			if !IsRetryable(err) {
				return err
			}
			lastErr = err
			continue
		}

		err := fn()
		if err == nil || !IsRetryable(err) {
			return err
		}
		lastErr = err
		c.logger.Debug("tcp: attempt failed", "attempt", attempt, "err", err)
	}
	return errors.Wrapf(lastErr, "tcp: giving up after %d attempts", p.MaxAttempts)
}
