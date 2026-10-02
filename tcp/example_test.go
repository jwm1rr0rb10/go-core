package tcp_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/jwm1rr0rb10/go-core/tcp"
)

// A framed echo server with a connection limit, an idle timeout and
// graceful shutdown.
func ExampleServer() {
	srv, _ := tcp.NewServer("127.0.0.1:0", func(ctx context.Context, conn net.Conn) {
		var buf []byte
		for {
			var err error
			if buf, err = tcp.ReadFrame(conn, buf[:0], 1<<20); err != nil {
				return // EOF, idle timeout or shutdown
			}
			if err := tcp.WriteFrame(conn, buf, 1<<20); err != nil {
				return
			}
		}
	}, tcp.WithMaxConnections(10_000), tcp.WithIdleTimeout(time.Minute))
	if err := srv.Start(); err != nil {
		panic(err)
	}

	ctx := context.Background()
	client, _ := tcp.Dial(ctx, srv.Addr().String())
	_ = client.WriteFrame(ctx, []byte("ping"))
	reply, _ := client.ReadFrame(ctx, nil)
	fmt.Println(string(reply))

	_ = client.Close()
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	fmt.Println(srv.Shutdown(shutdownCtx))
	// Output:
	// ping
	// <nil>
}

// Do retries a whole request/response exchange with reconnects and
// exponential backoff.
func ExampleClient_Do() {
	srv, _ := tcp.NewServer("127.0.0.1:0", func(_ context.Context, conn net.Conn) {
		_, _ = io.Copy(conn, conn)
	})
	_ = srv.Start()
	defer srv.Close()

	client, _ := tcp.NewClient(srv.Addr().String(),
		tcp.WithRetryPolicy(tcp.RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second}))
	defer client.Close()

	var reply []byte
	err := client.Do(context.Background(), func(ctx context.Context, c *tcp.Client) error {
		if err := c.WriteFrame(ctx, []byte("hello")); err != nil {
			return err
		}
		var err error
		reply, err = c.ReadFrame(ctx, nil)
		return err
	})
	fmt.Println(string(reply), err)
	// Output: hello <nil>
}

func ExamplePool() {
	srv, _ := tcp.NewServer("127.0.0.1:0", func(_ context.Context, conn net.Conn) {
		_, _ = io.Copy(conn, conn)
	})
	_ = srv.Start()
	defer srv.Close()

	pool, _ := tcp.NewPool(func(ctx context.Context) (*tcp.Client, error) {
		return tcp.Dial(ctx, srv.Addr().String())
	}, tcp.WithMaxActive(64), tcp.WithMaxIdle(16), tcp.WithPoolIdleTimeout(time.Minute))
	defer pool.Close()

	ctx := context.Background()
	for range 3 {
		c, err := pool.Get(ctx)
		if err != nil {
			panic(err)
		}
		if err := c.Write(ctx, []byte("x")); err != nil {
			pool.Discard(c) // broken: close it and free the slot
			continue
		}
		_, _ = c.Read(ctx)
		pool.Put(c)
	}
	st := pool.Stats()
	fmt.Println("created:", st.Misses, "reused:", st.Hits)
	// Output: created: 1 reused: 2
}

func ExampleRateLimiter_Check() {
	rl := tcp.NewRateLimiter(
		tcp.WithConnectionRate(1, 2), // 1/s sustained, burst of 2, then PoW
		tcp.WithBanRate(1, 3),        // beyond 3 in a burst: ban
		tcp.WithPoWDifficulty(16, 24),
	)
	defer rl.Stop()

	peer := netip.MustParseAddr("198.51.100.7")
	for range 4 {
		d, bits := rl.Check(peer)
		fmt.Println(d, bits)
	}
	// Output:
	// allow 0
	// allow 0
	// require-pow 16
	// deny 0
}

// Use the limiter as server middleware. With WithPoWHandshake, clients call
// AnswerPoW right after dialing.
func ExampleRateLimiter_Middleware() {
	rl := tcp.NewRateLimiter(tcp.WithPoWHandshake(true), tcp.WithPoWDifficulty(8, 12))
	defer rl.Stop()
	srv, _ := tcp.NewServer("127.0.0.1:0", func(_ context.Context, conn net.Conn) {
		_, _ = conn.Write([]byte("welcome"))
	}, tcp.WithMiddleware(rl.Middleware()))
	_ = srv.Start()
	defer srv.Close()

	conn, _ := net.Dial("tcp", srv.Addr().String())
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tcp.AnswerPoW(ctx, conn); err != nil {
		panic(err)
	}
	msg, _ := io.ReadAll(conn)
	fmt.Println(string(msg))
	// Output: welcome
}

func ExampleSolvePoW() {
	ch, _ := tcp.NewPoWChallenge(12, time.Minute)
	nonce, err := tcp.SolvePoW(context.Background(), ch)
	fmt.Println(err, ch.Verify(nonce))
	// Output: <nil> true
}

func ExampleWriteFrame() {
	var wire bytes.Buffer
	_ = tcp.WriteFrame(&wire, []byte("hello"), 0)
	fmt.Printf("% x\n", wire.Bytes()[:tcp.FrameHeaderSize])

	msg, _ := tcp.ReadFrame(&wire, nil, 0)
	fmt.Println(string(msg))
	// Output:
	// 00 00 00 05
	// hello
}
