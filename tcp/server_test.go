package tcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewServerValidation(t *testing.T) {
	if _, err := NewServer("127.0.0.1:0", nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
	s, err := NewServer("", echoHandler)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestServerEchoAndStats(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s)
	ctx := ctxTimeout(t, 2*time.Second)

	msg := []byte("hello")
	if err := c.Write(ctx, msg); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	if err := c.ReadFull(ctx, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("got %q", got)
	}

	st := s.Stats()
	if st.ActiveConnections != 1 || st.TotalConnections != 1 {
		t.Fatalf("unexpected stats %+v", st)
	}
	if st.BytesRead != 5 || st.BytesWritten != 5 {
		t.Fatalf("bytes: %+v", st)
	}
	if st.LastActivity.IsZero() {
		t.Fatal("last activity not set")
	}

	_ = c.Close()
	waitFor(t, time.Second, func() bool { return s.Stats().ActiveConnections == 0 })
	if st := s.Stats(); st.BytesRead != 5 || st.BytesWritten != 5 {
		t.Fatalf("closed-connection bytes lost: %+v", st)
	}
}

func TestServerShutdownGraceful(t *testing.T) {
	var cancelled atomic.Bool
	started := make(chan struct{})
	s := startServer(t, func(ctx context.Context, conn net.Conn) {
		close(started)
		<-ctx.Done()
		cancelled.Store(true)
	})
	dialClient(t, s)
	<-started

	if err := s.Shutdown(ctxTimeout(t, 2*time.Second)); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if !cancelled.Load() {
		t.Fatal("handler context was not cancelled")
	}
	if s.Addr() != nil {
		t.Fatal("listener still registered")
	}
	if err := s.Start(); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("restart: %v", err)
	}
}

func TestServerShutdownForcesStuckHandlers(t *testing.T) {
	started := make(chan struct{})
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		close(started)
		_, _ = io.Copy(io.Discard, conn) // ignores ctx; returns only when conn closes
	})
	dialClient(t, s)
	<-started

	err := s.Shutdown(ctxTimeout(t, 50*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	waitFor(t, time.Second, func() bool { return s.Stats().ActiveConnections == 0 })
}

func TestServerStopWithTimeout(t *testing.T) {
	started := make(chan struct{})
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		close(started)
		_, _ = io.Copy(io.Discard, conn)
	})
	dialClient(t, s)
	<-started
	if err := s.StopWithTimeout(20 * time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestServeReturnsErrServerClosed(t *testing.T) {
	s, _ := NewServer("", echoHandler)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ln) }()
	waitFor(t, time.Second, func() bool { return s.Addr() != nil })
	_ = s.Close()
	if err := <-done; !errors.Is(err, ErrServerClosed) {
		t.Fatalf("serve: %v", err)
	}
	if err := s.Serve(ln); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("serve after close: %v", err)
	}
}

func TestServerRecoversHandlerPanic(t *testing.T) {
	var calls atomic.Int32
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		echoHandler(context.Background(), conn)
	})
	ctx := ctxTimeout(t, 2*time.Second)

	c1 := dialClient(t, s)
	if _, err := c1.Read(ctx); err == nil {
		t.Fatal("expected the panicking connection to be closed")
	}

	c2 := dialClient(t, s)
	if err := c2.Write(ctx, []byte("ok")); err != nil {
		t.Fatal(err)
	}
	if got, err := c2.Read(ctx); err != nil || string(got) != "ok" {
		t.Fatalf("server unusable after panic: %q %v", got, err)
	}
}

func TestServerIdleTimeout(t *testing.T) {
	s := startServer(t, echoHandler, WithIdleTimeout(80*time.Millisecond))
	c := dialClient(t, s, WithTimeouts(2*time.Second, time.Second))
	ctx := ctxTimeout(t, 2*time.Second)

	// Activity keeps the connection alive past the timeout.
	for range 4 {
		if err := c.Write(ctx, []byte("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Read(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Silence closes it.
	start := time.Now()
	if _, err := c.Read(ctx); err == nil {
		t.Fatal("expected idle connection to be closed")
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("idle close took %v", waited)
	}
}

func TestServerManualDeadlineOverridesIdle(t *testing.T) {
	got := make(chan error, 1)
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		_, err := conn.Read(make([]byte, 1))
		got <- err
	}, WithIdleTimeout(20*time.Millisecond))
	c := dialClient(t, s)

	time.Sleep(100 * time.Millisecond) // well past the idle timeout
	if err := c.Write(context.Background(), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := <-got; err != nil {
		t.Fatalf("manual deadline was overridden by idle timeout: %v", err)
	}
}

func TestServerMiddleware(t *testing.T) {
	reject := errors.New("nope")
	var n atomic.Int32
	s := startServer(t, echoHandler, WithMiddleware(func(_ context.Context, conn net.Conn) (net.Conn, error) {
		if n.Add(1) == 1 {
			return nil, reject
		}
		return conn, nil
	}))
	ctx := ctxTimeout(t, 2*time.Second)

	c1 := dialClient(t, s)
	if _, err := c1.Read(ctx); err == nil {
		t.Fatal("rejected connection should be closed")
	}
	waitFor(t, time.Second, func() bool { return s.Stats().RejectedConnections == 1 })

	c2 := dialClient(t, s)
	if err := c2.Write(ctx, []byte("y")); err != nil {
		t.Fatal(err)
	}
	if b, err := c2.Read(ctx); err != nil || string(b) != "y" {
		t.Fatalf("got %q %v", b, err)
	}
}

func TestServerTLS(t *testing.T) {
	srvCfg, cliCfg, _, _ := selfSigned(t, "")
	s := startServer(t, echoHandler, WithServerTLS(srvCfg))
	c := dialClient(t, s, WithTLSClientConfig(cliCfg))
	ctx := ctxTimeout(t, 2*time.Second)
	if err := c.Write(ctx, []byte("secure")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	if err := c.ReadFull(ctx, b); err != nil || string(b) != "secure" {
		t.Fatalf("got %q %v", b, err)
	}
}
