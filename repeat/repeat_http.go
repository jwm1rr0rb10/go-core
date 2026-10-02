package repeat

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

// StatusError is the per-attempt error produced by [Transport] for a
// response whose status code is retryable. It is visible to [WithRetryIf],
// [WithRetryPolicy] and [WithOnRetry]; RoundTrip itself never returns it —
// when retries run out, the last response is returned with a nil error.
type StatusError struct {
	StatusCode int
	retryAfter time.Duration
}

// Error implements error.
func (e *StatusError) Error() string {
	return "repeat: retryable HTTP status " + strconv.Itoa(e.StatusCode) + " " + http.StatusText(e.StatusCode)
}

// RetryAfter returns the delay requested by the Retry-After header, or 0.
func (e *StatusError) RetryAfter() time.Duration { return e.retryAfter }

// DefaultRetryStatus reports whether a status code is worth retrying:
// 429 Too Many Requests, 502 Bad Gateway, 503 Service Unavailable and
// 504 Gateway Timeout. 500 is not retried by default: it usually signals a
// deterministic server bug rather than a transient condition.
func DefaultRetryStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// maxDrain bounds how much of a discarded response body is read so the
// connection can be reused.
const maxDrain = 64 << 10

// Transport is an [http.RoundTripper] that retries requests.
//
// Only idempotent requests are retried by default: GET, HEAD, OPTIONS,
// TRACE, PUT, DELETE, and any request carrying an Idempotency-Key or
// X-Idempotency-Key header. Requests with a body are retried only when
// req.GetBody is set (http.NewRequest sets it for *bytes.Buffer,
// *bytes.Reader and *strings.Reader bodies); otherwise they are sent once.
//
// Network errors and statuses accepted by RetryStatus are retried. A
// Retry-After header (seconds or HTTP date) raises the next delay. Discarded
// responses are drained (up to 64 KiB) and closed so connections are reused.
// When retries run out the last response is returned as is.
//
// http.Client.Timeout covers the whole sequence of attempts; use the
// request context or [WithMaxElapsed] to bound it explicitly.
type Transport struct {
	// Base performs the individual attempts. nil means http.DefaultTransport.
	Base http.RoundTripper
	// RetryStatus decides which status codes are retried. nil means
	// [DefaultRetryStatus].
	RetryStatus func(code int) bool
	// RetryNonIdempotent enables retries of POST, PATCH and other
	// non-idempotent requests. Only enable it when the server deduplicates.
	RetryNonIdempotent bool

	retrier *Retrier
	err     error
}

// NewTransport returns a [Transport] over base using the retry options.
// Invalid options make every RoundTrip fail with [ErrInvalidConfig].
func NewTransport(base http.RoundTripper, opts ...Option) *Transport {
	r, err := New(opts...)
	return &Transport{Base: base, retrier: r, err: err}
}

// NewClient returns a shallow copy of base (nil means a client with a 30s
// timeout) whose transport is wrapped in a retrying [Transport].
func NewClient(base *http.Client, opts ...Option) *http.Client {
	var c http.Client
	if base != nil {
		c = *base
	} else {
		c.Timeout = 30 * time.Second
	}
	c.Transport = NewTransport(c.Transport, opts...)
	return &c
}

// RoundTrip implements [http.RoundTripper].
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.err != nil {
		closeBody(req)
		return nil, t.err
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if !t.retryable(req) {
		return base.RoundTrip(req)
	}
	r := t.retrier
	if r == nil {
		r, _ = New()
	}
	retryStatus := t.RetryStatus
	if retryStatus == nil {
		retryStatus = DefaultRetryStatus
	}

	var last *http.Response
	err := r.exec(req.Context(), func(ctx context.Context, attempt int) error {
		attemptReq := req
		if attempt > 0 {
			attemptReq = req.Clone(ctx)
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return Permanent(err)
				}
				attemptReq.Body = body
			}
		}
		resp, err := base.RoundTrip(attemptReq)
		if err != nil {
			if ctx.Err() != nil {
				return Permanent(err)
			}
			return err
		}
		last = resp
		if !retryStatus(resp.StatusCode) {
			return nil
		}
		return &StatusError{
			StatusCode: resp.StatusCode,
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}, func() {
		if last != nil {
			drain(last)
			last = nil
		}
	})
	if last != nil {
		// Success, or a retryable status that we are not going to retry
		// any more: hand the response to the caller.
		return last, nil
	}
	return nil, err
}

func (t *Transport) retryable(req *http.Request) bool {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	if t.RetryNonIdempotent {
		return true
	}
	switch req.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	h := req.Header
	return h.Get("Idempotency-Key") != "" || h.Get("X-Idempotency-Key") != ""
}

// parseRetryAfter parses a Retry-After value (delay-seconds or HTTP-date).
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		if secs > int64(24*time.Hour/time.Second) {
			secs = int64(24 * time.Hour / time.Second)
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(0, t.Sub(now))
	}
	return 0
}

func drain(resp *http.Response) {
	_, _ = io.CopyN(io.Discard, resp.Body, maxDrain)
	_ = resp.Body.Close()
}

func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}
