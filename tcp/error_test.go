package tcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestConnectionErrorUnwrap(t *testing.T) {
	root := errors.New("root")
	err := error(&ConnectionError{Op: "write", Err: root, IsRetryable: true})
	var ce *ConnectionError
	if !errors.As(err, &ce) || !errors.Is(err, root) || ce.Error() != "tcp: write: root" {
		t.Fatalf("bad error: %v", err)
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("plain"), false},
		{io.EOF, true},
		{io.ErrUnexpectedEOF, true},
		{net.ErrClosed, true},
		{ErrTimeout, true},
		{ErrConnectionClosed, true},
		{os.ErrDeadlineExceeded, true},
		{&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, true},
		{fmt.Errorf("x: %w", syscall.ECONNREFUSED), true},
		{syscall.EPIPE, true},
		{context.Canceled, false},
		{context.DeadlineExceeded, false},
		{ErrClientClosed, false},
		{ErrFrameTooLarge, false},
		{&ConnectionError{Op: "x", Err: errors.New("y"), IsRetryable: true}, true},
		{&ConnectionError{Op: "x", Err: io.EOF, IsRetryable: false}, false},
		{wrapError("read", syscall.ECONNRESET), true},
	}
	for _, tc := range cases {
		if got := IsRetryable(tc.err); got != tc.want {
			t.Errorf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
