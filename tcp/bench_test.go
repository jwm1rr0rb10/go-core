package tcp

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/netip"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func BenchmarkVerifyPoW(b *testing.B) {
	ch, _ := NewPoWChallenge(8, 0)
	nonce, _ := SolvePoW(context.Background(), ch)
	b.ReportAllocs()
	for b.Loop() {
		if !ch.Verify(nonce) {
			b.Fatal("invalid")
		}
	}
}

func BenchmarkSolvePoW16(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ch, _ := NewPoWChallenge(16, 0)
		if _, err := SolvePoW(context.Background(), ch); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRateLimiterCheck(b *testing.B) {
	rl := NewRateLimiter(WithCleanupInterval(0), WithConnectionRate(1e9, 1e9), WithBanRate(1e9, 1e9))
	defer rl.Stop()
	b.Run("single-peer", func(b *testing.B) {
		addr := netip.MustParseAddr("192.0.2.1")
		b.ReportAllocs()
		for b.Loop() {
			rl.Check(addr)
		}
	})
	b.Run("parallel-many-peers", func(b *testing.B) {
		var seed atomic.Uint32
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			base := seed.Add(1) << 12
			i := uint32(0)
			for pb.Next() {
				v := base + i&0xfff
				rl.Check(netip.AddrFrom4([4]byte{10, byte(v >> 16), byte(v >> 8), byte(v)}))
				i++
			}
		})
	})
}

func BenchmarkFrameRoundTrip(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 512)
	var buf bytes.Buffer
	var out []byte
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		buf.Reset()
		_ = WriteFrame(&buf, payload, 0)
		out, _ = ReadFrame(&buf, out[:0], 0)
	}
}

func BenchmarkIsRetryable(b *testing.B) {
	err := wrapError("read", syscall.ECONNRESET)
	b.ReportAllocs()
	for b.Loop() {
		if !IsRetryable(err) {
			b.Fatal("expected retryable")
		}
	}
}

// BenchmarkClientEcho measures a framed request/response round trip over
// loopback through Server and Client.
func BenchmarkClientEcho(b *testing.B) {
	s := startServer(b, func(_ context.Context, conn netConn) {
		var buf []byte
		for {
			var err error
			if buf, err = ReadFrame(conn, buf[:0], 0); err != nil {
				return
			}
			if WriteFrame(conn, buf, 0) != nil {
				return
			}
		}
	}, WithIdleTimeout(time.Minute))
	c := dialClient(b, s)
	ctx := context.Background()
	payload := bytes.Repeat([]byte("x"), 128)
	var out []byte
	b.ReportAllocs()
	for b.Loop() {
		if err := c.WriteFrame(ctx, payload); err != nil {
			b.Fatal(err)
		}
		var err error
		if out, err = c.ReadFrame(ctx, out[:0]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkServerConnRead measures the per-read overhead of the server's
// connection wrapper (stats + sliding idle deadline).
func BenchmarkServerConnRead(b *testing.B) {
	sc := newServerConn(nopConn{}, time.Minute, time.Now())
	p := make([]byte, 64)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = sc.Read(p)
	}
}

type netConn = net.Conn

type nopConn struct{ netConn }

func (nopConn) Read(p []byte) (int, error)       { return len(p), nil }
func (nopConn) SetReadDeadline(time.Time) error  { return nil }
func (nopConn) Write(p []byte) (int, error)      { return len(p), nil }
func (nopConn) Close() error                     { return nil }
func (nopConn) SetWriteDeadline(time.Time) error { return nil }

var _ io.Reader = nopConn{}
