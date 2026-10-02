package tcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestClientRoundTripAndStats(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithBufferSize(16))
	ctx := ctxTimeout(t, 2*time.Second)

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect must be idempotent: %v", err)
	}
	if err := c.Write(ctx, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := c.ReadInto(ctx, buf)
	if err != nil || string(buf[:n]) != "abc" {
		t.Fatalf("ReadInto: %q %v", buf[:n], err)
	}
	st := c.Stats()
	if st.BytesRead != 3 || st.BytesWritten != 3 || st.LastActivity.IsZero() {
		t.Fatalf("stats: %+v", st)
	}
	if c.RemoteAddr() == nil || c.LocalAddr() == nil || !c.Connected() {
		t.Fatal("addresses should be set while connected")
	}
}

func TestClientFrames(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithMaxFrameSize(1024))
	ctx := ctxTimeout(t, 2*time.Second)

	var buf []byte
	for _, msg := range []string{"one", "", "three"} {
		if err := c.WriteFrame(ctx, []byte(msg)); err != nil {
			t.Fatal(err)
		}
		var err error
		buf, err = c.ReadFrame(ctx, buf[:0])
		if err != nil || string(buf) != msg {
			t.Fatalf("frame %q: got %q %v", msg, buf, err)
		}
	}
	if err := c.WriteFrame(ctx, make([]byte, 2048)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestClientCloseIsFinal(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s)
	ctx := ctxTimeout(t, 2*time.Second)

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	for name, err := range map[string]error{
		"reconnect": c.Reconnect(ctx),
		"connect":   c.Connect(ctx),
		"write":     c.Write(ctx, []byte("x")),
		"retry":     c.WriteWithRetry(ctx, []byte("x")),
	} {
		if !errors.Is(err, ErrClientClosed) {
			t.Errorf("%s after Close: %v", name, err)
		}
	}
	if c.RemoteAddr() != nil || c.LocalAddr() != nil {
		t.Error("addresses should be nil after Close")
	}
}

func TestClientCloseInterruptsRead(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithTimeouts(0, 0))
	errc := make(chan error, 1)
	go func() {
		_, err := c.Read(context.Background())
		errc <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = c.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, ErrClientClosed) {
			t.Fatalf("expected ErrClientClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read was not interrupted by Close")
	}
}

func TestClientContextCancelInterruptsRead(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithTimeouts(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err := c.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if IsRetryable(err) {
		t.Fatal("cancellation must not be retryable")
	}
	// The connection is still usable afterwards.
	ctx2 := ctxTimeout(t, time.Second)
	if err := c.Write(ctx2, []byte("z")); err != nil {
		t.Fatal(err)
	}
	if b, err := c.Read(ctx2); err != nil || string(b) != "z" {
		t.Fatalf("after cancel: %q %v", b, err)
	}
}

func TestClientReadTimeout(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithTimeouts(30*time.Millisecond, time.Second))
	_, err := c.Read(context.Background())
	if !errors.Is(err, ErrTimeout) || !IsRetryable(err) {
		t.Fatalf("expected retryable ErrTimeout, got %v", err)
	}
}

func TestClientContextDeadline(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, WithTimeouts(time.Minute, time.Minute))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.Read(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestClientPeerClose(t *testing.T) {
	s := startServer(t, func(_ context.Context, conn net.Conn) {})
	c := dialClient(t, s)
	_, err := c.Read(ctxTimeout(t, time.Second))
	if !errors.Is(err, io.EOF) || !IsRetryable(err) {
		t.Fatalf("expected retryable EOF, got %v", err)
	}
}

func TestClientConcurrentReadWrite(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s)
	ctx := ctxTimeout(t, 5*time.Second)
	payload := bytes.Repeat([]byte("x"), 64)
	const rounds = 200
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, rounds*len(payload))
		done <- c.ReadFull(ctx, buf)
	}()
	for range rounds {
		if err := c.Write(ctx, payload); err != nil {
			t.Fatal(err)
		}
		_ = c.Stats()
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
