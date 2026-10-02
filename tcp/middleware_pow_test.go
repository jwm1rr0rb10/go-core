package tcp

import (
	"context"
	"net"
	"testing"
	"time"
)

// powServer starts an echo server behind a limiter whose soft burst is 1, so
// the second connection from 127.0.0.1 must solve a small challenge.
func powServer(t *testing.T, opts ...RateLimiterOption) (*Server, *RateLimiter) {
	t.Helper()
	opts = append([]RateLimiterOption{
		WithConnectionRate(0.001, 1), WithBanRate(1000, 1000),
		WithPoWDifficulty(8, 10), WithPoWTimeout(2 * time.Second),
		WithCleanupInterval(0),
	}, opts...)
	rl := NewRateLimiter(opts...)
	t.Cleanup(rl.Stop)
	s := startServer(t, echoHandler, WithMiddleware(rl.Middleware()))
	return s, rl
}

func rawDial(t *testing.T, s *Server) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", s.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	return conn
}

func echoCheck(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := readFull(conn, buf); err != nil || string(buf) != msg {
		t.Fatalf("echo %q: got %q %v", msg, buf, err)
	}
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := conn.Read(buf[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func TestPoWHandshakeFlow(t *testing.T) {
	s, _ := powServer(t, WithPoWHandshake(true))
	ctx := ctxTimeout(t, 5*time.Second)

	first := rawDial(t, s)
	if err := AnswerPoW(ctx, first); err != nil {
		t.Fatalf("unthrottled handshake: %v", err)
	}
	echoCheck(t, first, "first")

	second := rawDial(t, s)
	if err := AnswerPoW(ctx, second); err != nil {
		t.Fatalf("pow handshake: %v", err)
	}
	echoCheck(t, second, "second")
}

func TestPoWWithoutHandshakeKeepsPipelinedData(t *testing.T) {
	s, _ := powServer(t)
	ctx := ctxTimeout(t, 5*time.Second)

	first := rawDial(t, s)
	echoCheck(t, first, "plain") // no extra bytes when not throttled

	second := rawDial(t, s)
	line, err := readLineUnbuffered(second)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := ParsePoWChallenge(line)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := SolvePoW(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	// Solution and application data in a single write.
	if _, err := second.Write([]byte("SOLUTION nonce=" + nonce + "\nPIPELINED")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("PIPELINED"))
	if _, err := readFull(second, buf); err != nil || string(buf) != "PIPELINED" {
		t.Fatalf("pipelined data: %q %v", buf, err)
	}
}

func TestAnswerPoWHonoursContext(t *testing.T) {
	s, _ := powServer(t, WithPoWHandshake(true), WithPoWDifficulty(MaxPoWBits, MaxPoWBits))
	ctx := ctxTimeout(t, 3*time.Second)
	_ = AnswerPoW(ctx, rawDial(t, s)) // consume the free token

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := AnswerPoW(short, rawDial(t, s)); err == nil {
		t.Fatal("expected the 40-bit challenge to time out")
	}
}
