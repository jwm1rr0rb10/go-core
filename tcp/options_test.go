package tcp

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestServerOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer("127.0.0.1:0", echoHandler,
		WithServerLogger(logger), WithServerLogger(nil),
		WithIdleTimeout(time.Second), WithIdleTimeout(-1),
		WithServerTimeout(2*time.Second),
		WithMaxConnections(10), WithMiddleware(nil),
		WithListenConfig(net.ListenConfig{KeepAlive: time.Second}),
		WithBaseContext(context.Background()), WithBaseContext(nil), //nolint:staticcheck // nil is ignored
	)
	if err != nil {
		t.Fatal(err)
	}
	if s.logger != logger || s.idleTimeout != 2*time.Second || s.maxConns.Load() != 10 || len(s.middleware) != 0 {
		t.Fatalf("options not applied: %+v", s)
	}
}

func TestClientOptions(t *testing.T) {
	c, err := NewClient("127.0.0.1:1",
		WithTimeouts(time.Second, -time.Second), WithDialTimeout(3*time.Second),
		WithBufferSize(128), WithBufferSize(0), WithMaxFrameSize(99), WithMaxFrameSize(0),
		WithClientLogger(nil), WithTLSClientConfig(ClientTLSConfig(false)),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 0}))
	if err != nil {
		t.Fatal(err)
	}
	if c.readTimeout != time.Second || c.writeTimeout != 0 || c.dialer.Timeout != 3*time.Second ||
		c.bufferSize != 128 || c.maxFrameSize != 99 || c.tlsConfig == nil || c.retry.MaxAttempts != 1 {
		t.Fatalf("options not applied: %+v", c)
	}
	c2, _ := NewClient("x:1", WithDialer(net.Dialer{Timeout: time.Millisecond}))
	if c2.dialer.Timeout != time.Millisecond {
		t.Fatal("dialer not applied")
	}
}

func TestPoolOptions(t *testing.T) {
	p, err := NewPool(func(context.Context) (*Client, error) { return nil, nil },
		WithMaxIdle(-1), WithMaxActive(3), WithPoolIdleTimeout(time.Second),
		WithPoolMaxLifetime(time.Minute), WithPoolHealthCheck(false), WithPoolLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	if p.maxIdle != 0 || cap(p.sem) != 3 || p.idleTimeout != time.Second || p.maxLifetime != time.Minute || p.healthCheck {
		t.Fatalf("options not applied: %+v", p)
	}
	p2, _ := NewPool(func(context.Context) (*Client, error) { return nil, nil }, WithMaxActive(3), WithMaxActive(0))
	if p2.sem != nil {
		t.Fatal("WithMaxActive(0) should mean unlimited")
	}
}

func TestRateLimiterOptions(t *testing.T) {
	rl := NewRateLimiter(WithCleanupInterval(0),
		WithConnectionRate(0, 0), WithBanRate(-1, 5), WithBanDuration(0),
		WithPoWTimeout(0), WithDifficultyDecay(0), WithMaxTrackedPeers(0),
		WithPoWDifficulty(30, 10), WithIPv6PrefixLen(200), WithClock(nil), WithRateLimiterLogger(nil))
	defer rl.Stop()
	c := rl.cfg
	if c.softRate != 10 || c.hardRate != 20 || c.banDuration != time.Minute || c.powTimeout != 10*time.Second ||
		c.minBits != 30 || c.maxBits != 30 || c.v6PrefixLen != 128 || c.now == nil {
		t.Fatalf("defaults/clamping wrong: %+v", c)
	}
}
