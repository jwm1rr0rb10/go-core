package tcp

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newTestLimiter(t *testing.T, clk *fakeClock, opts ...RateLimiterOption) *RateLimiter {
	t.Helper()
	opts = append([]RateLimiterOption{WithClock(clk.Now), WithCleanupInterval(0)}, opts...)
	rl := NewRateLimiter(opts...)
	t.Cleanup(rl.Stop)
	return rl
}

var ip1 = netip.MustParseAddr("192.0.2.1")

func TestRateLimiterSoftHardBan(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk,
		WithConnectionRate(1, 3), WithBanRate(1, 5), WithBanDuration(time.Minute))

	for i := range 3 {
		if d, _ := rl.Check(ip1); d != Allow {
			t.Fatalf("attempt %d: %v", i, d)
		}
	}
	for i := range 2 {
		if d, bits := rl.Check(ip1); d != RequirePoW || bits == 0 {
			t.Fatalf("attempt %d: %v %d", i+3, d, bits)
		}
	}
	if d, _ := rl.Check(ip1); d != Deny {
		t.Fatalf("expected ban, got %v", d)
	}
	clk.Advance(30 * time.Second)
	if d, _ := rl.Check(ip1); d != Deny {
		t.Fatal("ban should still be active")
	}
	clk.Advance(31 * time.Second)
	if d, _ := rl.Check(ip1); d != Allow {
		t.Fatal("ban should have expired with full buckets")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithConnectionRate(10, 1), WithBanRate(100, 100))
	if d, _ := rl.Check(ip1); d != Allow {
		t.Fatal("first")
	}
	if d, _ := rl.Check(ip1); d != RequirePoW {
		t.Fatal("bucket should be empty")
	}
	clk.Advance(100 * time.Millisecond) // one token at 10/s
	if d, _ := rl.Check(ip1); d != Allow {
		t.Fatal("token should have been refilled")
	}
}

func TestRateLimiterDifficultyGrowsAndDecays(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithConnectionRate(0.0001, 1), WithBanRate(1000, 1000),
		WithPoWDifficulty(10, 13), WithDifficultyDecay(time.Second))
	rl.Check(ip1) // consume the only soft token

	want := []int{10, 11, 12, 13, 13}
	for i, w := range want {
		if _, bits := rl.Check(ip1); bits != w {
			t.Fatalf("challenge %d: bits %d, want %d", i, bits, w)
		}
	}
	clk.Advance(2 * time.Second) // decays two bits: 13 → 11, next challenge 12
	if _, bits := rl.Check(ip1); bits != 12 {
		t.Fatalf("after decay: %d", bits)
	}
	clk.Advance(time.Hour)
	if _, bits := rl.Check(ip1); bits != 10 {
		t.Fatalf("after full decay: %d", bits)
	}
}

func TestRateLimiterIPv6Grouping(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithConnectionRate(1, 1), WithBanRate(100, 100))
	a := netip.MustParseAddr("2001:db8:1:2::1")
	b := netip.MustParseAddr("2001:db8:1:2:ffff::9")
	c := netip.MustParseAddr("2001:db8:1:3::1")
	rl.Check(a)
	if d, _ := rl.Check(b); d != RequirePoW {
		t.Fatal("same /64 should share a bucket")
	}
	if d, _ := rl.Check(c); d != Allow {
		t.Fatal("different /64 should have its own bucket")
	}

	exact := newTestLimiter(t, clk, WithConnectionRate(1, 1), WithIPv6PrefixLen(128))
	exact.Check(a)
	if d, _ := exact.Check(b); d != Allow {
		t.Fatal("with /128 addresses are independent")
	}
}

func TestRateLimiterIPv4Mapped(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithConnectionRate(1, 1))
	rl.Check(netip.MustParseAddr("192.0.2.7"))
	if d, _ := rl.Check(netip.MustParseAddr("::ffff:192.0.2.7")); d != RequirePoW {
		t.Fatal("IPv4-mapped address must map to the same peer")
	}
}

func TestRateLimiterSaturation(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithMaxTrackedPeers(rateLimiterShards), WithPoWDifficulty(8, 20))
	saturated := 0
	for i := range 2000 {
		addr := netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
		if d, bits := rl.Check(addr); d == RequirePoW && bits == 20 {
			saturated++
		}
	}
	if saturated == 0 {
		t.Fatal("expected newcomers to pay max difficulty once shards are full")
	}
	if rl.Len() > 2*rateLimiterShards {
		t.Fatalf("memory bound violated: %d peers", rl.Len())
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	clk := newFakeClock()
	rl := newTestLimiter(t, clk, WithBanRate(1, 1), WithBanDuration(time.Hour))
	rl.Check(ip1)
	rl.Check(ip1) // banned
	other := netip.MustParseAddr("192.0.2.2")
	rl.Check(other)
	clk.Advance(10 * time.Minute)
	if n := rl.Cleanup(); n != 1 {
		t.Fatalf("removed %d, want 1 (banned peer must stay)", n)
	}
	if d, _ := rl.Check(ip1); d != Deny {
		t.Fatal("ban lost by cleanup")
	}
}

func TestRateLimiterStopIdempotent(t *testing.T) {
	rl := NewRateLimiter(WithCleanupInterval(time.Millisecond))
	time.Sleep(5 * time.Millisecond)
	rl.Stop()
	rl.Stop()
}
