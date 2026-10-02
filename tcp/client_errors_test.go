package tcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(""); err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestClientNotConnected(t *testing.T) {
	c, _ := NewClient("127.0.0.1:1")
	ctx := context.Background()
	if _, err := c.Read(ctx); !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("read: %v", err)
	}
	if err := c.Write(ctx, nil); !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("write: %v", err)
	}
	if c.Connected() || c.RemoteAddr() != nil {
		t.Fatal("should not be connected")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close of never-connected client: %v", err)
	}
}

func TestClientDialRefusedIsRetryable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c, _ := NewClient(addr, WithDialTimeout(time.Second))
	err = c.Connect(context.Background())
	var ce *ConnectionError
	if !errors.As(err, &ce) || ce.Op != "dial" || !IsRetryable(err) {
		t.Fatalf("expected retryable dial error, got %v", err)
	}
}

func TestClientCancelledContext(t *testing.T) {
	c, _ := NewClient("127.0.0.1:1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Connect(ctx); err == nil || IsRetryable(err) {
		t.Fatalf("expected non-retryable error, got %v", err)
	}
	if _, err := c.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("read: %v", err)
	}
}

func TestClientTLSVerificationFailure(t *testing.T) {
	srvCfg, _, _, _ := selfSigned(t, "")
	s := startServer(t, echoHandler, WithServerTLS(srvCfg))
	c, _ := NewClient(s.Addr().String(), WithTLSClientConfig(ClientTLSConfig(false)))
	if err := c.Connect(ctxTimeout(t, 2*time.Second)); err == nil {
		t.Fatal("expected certificate verification error")
	}
}
