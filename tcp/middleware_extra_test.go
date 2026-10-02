package tcp

import (
	"net"
	"net/netip"
	"sync"
	"testing"
)

func ip1Loopback() netip.Addr { return netip.MustParseAddr("127.0.0.1") }

func TestDecisionString(t *testing.T) {
	for d, want := range map[Decision]string{Allow: "allow", RequirePoW: "require-pow", Deny: "deny", 9: "unknown"} {
		if d.String() != want {
			t.Errorf("%d: %s", d, d.String())
		}
	}
}

type addrConn struct {
	net.Conn
	addr net.Addr
}

func (c addrConn) RemoteAddr() net.Addr { return c.addr }

func TestRemoteIP(t *testing.T) {
	tcp := addrConn{addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.9"), Port: 1}}
	if ip, ok := remoteIP(tcp); !ok || ip.Unmap() != netip.MustParseAddr("192.0.2.9") {
		t.Fatalf("tcp addr: %v %v", ip, ok)
	}
	udp := addrConn{addr: &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 5}}
	if ip, ok := remoteIP(udp); !ok || ip != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("generic addr: %v %v", ip, ok)
	}
	unix := addrConn{addr: &net.UnixAddr{Name: "/tmp/x", Net: "unix"}}
	if _, ok := remoteIP(unix); ok {
		t.Fatal("unix address is not an IP")
	}
	if _, ok := remoteIP(addrConn{}); ok {
		t.Fatal("nil address")
	}
}

func TestRateLimiterMiddlewareRejectsNonIP(t *testing.T) {
	rl := NewRateLimiter(WithCleanupInterval(0))
	defer rl.Stop()
	if _, err := rl.Middleware()(t.Context(), addrConn{addr: &net.UnixAddr{Name: "x"}}); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := NewRateLimiter(WithCleanupInterval(0))
	defer rl.Stop()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 2000 {
				rl.Check(netip.AddrFrom4([4]byte{10, 0, byte(g), byte(i)}))
				if i%500 == 0 {
					rl.Cleanup()
					_ = rl.Len()
				}
			}
		})
	}
	wg.Wait()
}

func TestShardIndexSpread(t *testing.T) {
	var counts [rateLimiterShards]int
	for i := range 64 * 100 {
		k := peerKey(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}).As16())
		counts[shardIndex(&k)]++
	}
	for i, n := range counts {
		if n == 0 {
			t.Fatalf("shard %d never used", i)
		}
	}
}
