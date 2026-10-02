package tcp

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

var fastRetry = WithRetryPolicy(RetryPolicy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond})

func TestClientDoReconnects(t *testing.T) {
	var conns atomic.Int32
	s := startServer(t, func(ctx context.Context, conn net.Conn) {
		if conns.Add(1) == 1 {
			return // drop the first connection immediately
		}
		echoHandler(ctx, conn)
	})
	c, _ := NewClient(s.Addr().String(), fastRetry)
	t.Cleanup(func() { _ = c.Close() })
	ctx := ctxTimeout(t, 3*time.Second)

	var reply []byte
	err := c.Do(ctx, func(ctx context.Context, c *Client) error {
		if err := c.WriteFrame(ctx, []byte("ping")); err != nil {
			return err
		}
		var err error
		reply, err = c.ReadFrame(ctx, nil)
		return err
	})
	if err != nil || string(reply) != "ping" {
		t.Fatalf("Do: %q %v", reply, err)
	}
	if st := c.Stats(); st.Reconnects == 0 || st.RetryCount == 0 {
		t.Fatalf("expected a reconnect: %+v", st)
	}
}

func TestClientDoNonRetryableStops(t *testing.T) {
	s := startServer(t, echoHandler)
	c := dialClient(t, s, fastRetry)
	boom := errors.New("application error")
	calls := 0
	err := c.Do(context.Background(), func(context.Context, *Client) error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestClientDoGivesUp(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	c, _ := NewClient(addr, fastRetry, WithDialTimeout(200*time.Millisecond))
	err := c.Do(ctxTimeout(t, 3*time.Second), func(context.Context, *Client) error { return nil })
	if err == nil || !IsRetryable(err) {
		t.Fatalf("expected retryable give-up error, got %v", err)
	}
}

func TestWriteWithRetryAfterServerDrop(t *testing.T) {
	var conns atomic.Int32
	got := make(chan string, 1)
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		if conns.Add(1) == 1 {
			return
		}
		b := make([]byte, 5)
		if _, err := conn.Read(b); err == nil {
			got <- string(b)
		}
	})
	c := dialClient(t, s, fastRetry)
	waitFor(t, time.Second, func() bool { return s.Stats().ActiveConnections == 0 })
	ctx := ctxTimeout(t, 3*time.Second)
	// The first write may still succeed into the kernel buffer; keep writing
	// until the broken connection is detected and the client reconnects.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := c.WriteWithRetry(ctx, []byte("hello")); err != nil {
			t.Fatal(err)
		}
		select {
		case msg := <-got:
			if msg != "hello" {
				t.Fatalf("got %q", msg)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("write never reached the second connection")
		}
	}
}

func TestReadWithRetryTimeoutThenData(t *testing.T) {
	s := startServer(t, func(_ context.Context, conn net.Conn) {
		time.Sleep(60 * time.Millisecond)
		_, _ = conn.Write([]byte("late"))
		_, _ = conn.Read(make([]byte, 1))
	})
	c := dialClient(t, s, fastRetry, WithTimeouts(25*time.Millisecond, time.Second),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 10, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}))
	b, err := c.ReadWithRetry(ctxTimeout(t, 3*time.Second))
	if err != nil || string(b) != "late" {
		t.Fatalf("got %q %v", b, err)
	}
	if c.Stats().Reconnects != 0 {
		t.Fatal("read timeouts must not trigger reconnects")
	}
}
