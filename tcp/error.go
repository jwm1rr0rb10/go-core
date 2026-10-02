package tcp

import (
	"context"
	"io"
	"net"
	"syscall"

	"github.com/jwm1rr0rb10/go-errors"
)

const network = "tcp"

// Sentinel errors returned by this package. Compare with [errors.Is].
var (
	// ErrConnectionClosed is returned when an operation needs a connection
	// but the client is not connected (or the peer closed the connection).
	ErrConnectionClosed = errors.New("tcp: connection closed")
	// ErrTimeout is returned when a read or write deadline expires.
	ErrTimeout = errors.New("tcp: operation timeout")
	// ErrClientClosed is returned by every Client method after Close.
	ErrClientClosed = errors.New("tcp: client closed")
	// ErrServerClosed is returned by Server.Serve after Shutdown or Close.
	ErrServerClosed = errors.New("tcp: server closed")
	// ErrPoolClosed is returned by Pool.Get after Pool.Close.
	ErrPoolClosed = errors.New("tcp: pool closed")
	// ErrFrameTooLarge is returned when a frame exceeds the configured limit.
	ErrFrameTooLarge = errors.New("tcp: frame too large")
	// ErrBanned is returned by the rate-limit middleware for banned peers.
	ErrBanned = errors.New("tcp: peer banned")
	// ErrRateLimited is returned when a peer exceeds its rate and proof of
	// work is disabled or the limiter is saturated.
	ErrRateLimited = errors.New("tcp: rate limited")
	// ErrPoWFailed is returned when a proof-of-work solution is missing,
	// malformed, expired or does not meet the difficulty.
	ErrPoWFailed = errors.New("tcp: proof of work failed")
)

// ConnectionError describes a failed network operation.
type ConnectionError struct {
	// Op is the operation that failed: "dial", "read", "write", ...
	Op string
	// Err is the underlying error.
	Err error
	// IsRetryable reports whether repeating the operation (possibly after a
	// reconnect) may succeed.
	IsRetryable bool
}

// Error implements error.
func (e *ConnectionError) Error() string {
	return "tcp: " + e.Op + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ConnectionError) Unwrap() error { return e.Err }

func wrapError(op string, err error) error {
	return &ConnectionError{Op: op, Err: err, IsRetryable: classifyRetryable(err)}
}

// IsRetryable reports whether err is a transient network failure that may
// succeed after a reconnect: timeouts, resets, refused connections, broken
// pipes and unexpected EOFs. Context cancellation, [ErrClientClosed] and
// [ErrFrameTooLarge] are never retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if ce, ok := errors.AsType[*ConnectionError](err); ok {
		return ce.IsRetryable
	}
	return classifyRetryable(err)
}

func classifyRetryable(err error) bool {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, ErrClientClosed), errors.Is(err, ErrFrameTooLarge):
		return false
	case errors.Is(err, ErrTimeout), errors.Is(err, ErrConnectionClosed),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, net.ErrClosed),
		errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.ECONNABORTED), errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ETIMEDOUT), errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH):
		return true
	}
	ne, ok := errors.AsType[net.Error](err)
	return ok && ne.Timeout()
}

// isTimeout reports whether err is an I/O deadline error.
func isTimeout(err error) bool {
	ne, ok := errors.AsType[net.Error](err)
	return ok && ne.Timeout()
}
