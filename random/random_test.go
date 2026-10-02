package random_test

import (
	"errors"
	"math"
	"math/rand/v2"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/random"
)

func seeded() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func TestRandIntRange(t *testing.T) {
	for _, r := range []*rand.Rand{nil, seeded(), random.Secure()} {
		for range 1000 {
			v, err := random.RandInt(r, -5, 5)
			if err != nil || v < -5 || v >= 5 {
				t.Fatalf("RandInt = %d, %v", v, err)
			}
		}
	}
	if _, err := random.RandInt(nil, 10, 10); !errors.Is(err, random.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange, got %v", err)
	}
}

func TestRandIntFullRange(t *testing.T) {
	// max-min overflows int64; the old implementation panicked here.
	for range 1000 {
		if _, err := random.RandInt64(nil, math.MinInt64, math.MaxInt64); err != nil {
			t.Fatal(err)
		}
		if _, err := random.RandInt(nil, math.MinInt, math.MaxInt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRandIntCoversRange(t *testing.T) {
	seen := map[int]bool{}
	for range 1000 {
		v, _ := random.RandInt(nil, 0, 4)
		seen[v] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected all 4 values, got %v", seen)
	}
}

func TestRandFloat64(t *testing.T) {
	r := seeded()
	for range 1000 {
		f, err := random.RandFloat64(r, -1, 1)
		if err != nil || f < -1 || f >= 1 {
			t.Fatalf("RandFloat64 = %v, %v", f, err)
		}
	}
	for _, c := range [][2]float64{{1, 1}, {2, 1}, {math.NaN(), 1}, {0, math.Inf(1)}, {math.Inf(-1), 0}} {
		if _, err := random.RandFloat64(nil, c[0], c[1]); !errors.Is(err, random.ErrInvalidRange) {
			t.Errorf("RandFloat64(%v, %v): expected ErrInvalidRange, got %v", c[0], c[1], err)
		}
	}
}

func TestRandStringAndSecureString(t *testing.T) {
	for name, gen := range map[string]func(int, []byte) (string, error){
		"fast":   func(n int, set []byte) (string, error) { return random.RandString(nil, n, set) },
		"secure": random.SecureString,
	} {
		s, err := gen(1000, []byte("ab"))
		if err != nil || len(s) != 1000 || strings.Trim(s, "ab") != "" {
			t.Fatalf("%s: %q %v", name, s, err)
		}
		if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
			t.Fatalf("%s: alphabet not covered", name)
		}
		def, _ := gen(64, nil)
		if len(def) != 64 || strings.Trim(def, string(random.DefaultSet)) != "" {
			t.Fatalf("%s default set: %q", name, def)
		}
		if empty, err := gen(0, nil); err != nil || empty != "" {
			t.Fatalf("%s n=0: %q %v", name, empty, err)
		}
		if _, err := gen(-1, nil); !errors.Is(err, random.ErrNegativeCount) {
			t.Fatalf("%s n<0: %v", name, err)
		}
	}
}

func TestStringUniform(t *testing.T) {
	// 3-letter set: 256 % 3 != 0, so a naive modulo would be biased.
	fast, _ := random.RandString(nil, 300_000, []byte("abc"))
	secure, _ := random.SecureString(300_000, []byte("abc"))
	for name, s := range map[string]string{"fast": fast, "secure": secure} {
		for _, c := range "abc" {
			n := strings.Count(s, string(c))
			if n < 98_000 || n > 102_000 {
				t.Fatalf("%s: letter %c count %d is far from 100000", name, c, n)
			}
		}
	}
}

func TestRandStringLargeSet(t *testing.T) {
	set := make([]byte, 300) // > 256 takes the slow path
	for i := range set {
		set[i] = byte('a' + i%26)
	}
	s, err := random.RandString(nil, 50, set)
	if err != nil || len(s) != 50 {
		t.Fatalf("%q %v", s, err)
	}
	s, err = random.SecureString(50, set)
	if err != nil || len(s) != 50 {
		t.Fatalf("%q %v", s, err)
	}
}

func TestSecureIntTokenText(t *testing.T) {
	v, err := random.SecureInt(10, 20)
	if err != nil || v < 10 || v >= 20 {
		t.Fatalf("SecureInt = %d, %v", v, err)
	}
	v64, err := random.SecureInt64(-3, -1)
	if err != nil || v64 < -3 || v64 >= -1 {
		t.Fatalf("SecureInt64 = %d, %v", v64, err)
	}
	tok, err := random.SecureToken(16)
	if err != nil || len(tok) != 32 {
		t.Fatalf("SecureToken = %q, %v", tok, err)
	}
	if _, err := random.SecureToken(-1); err == nil {
		t.Fatal("expected error")
	}
	if txt := random.SecureText(); len(txt) != 26 {
		t.Fatalf("SecureText = %q", txt)
	}
}

func TestPick(t *testing.T) {
	v, err := random.Pick(seeded(), "a", "b", "c")
	if err != nil || !strings.Contains("abc", v) {
		t.Fatalf("Pick = %q, %v", v, err)
	}
	if _, err := random.Pick[int](nil); !errors.Is(err, random.ErrEmpty) {
		t.Fatalf("Pick empty: %v", err)
	}
	anyV, err := random.RandomCase(nil, 1, 2)
	if err != nil || (anyV != 1 && anyV != 2) {
		t.Fatalf("RandomCase = %v, %v", anyV, err)
	}
}

func TestRandomBool(t *testing.T) {
	trues := 0
	for range 10_000 {
		if random.RandomBool(nil) {
			trues++
		}
	}
	if trues < 4500 || trues > 5500 {
		t.Fatalf("RandomBool skewed: %d/10000", trues)
	}
}

func TestRandomTimeAndDate(t *testing.T) {
	lo := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hi := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for range 1000 {
		got, err := random.RandomTime(nil, lo, hi)
		if err != nil || got.Before(lo) || got.After(hi) {
			t.Fatalf("RandomTime = %v, %v", got, err)
		}
		d, err := random.RandomDate(nil, lo, hi)
		if err != nil || d.Before(lo) || d.After(hi) || d.Nanosecond() != 0 {
			t.Fatalf("RandomDate = %v, %v", d, err)
		}
	}
	if got, _ := random.RandomTime(nil, lo, lo); !got.Equal(lo) {
		t.Fatalf("degenerate range: %v", got)
	}
	if _, err := random.RandomTime(nil, hi, lo); !errors.Is(err, random.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange: %v", err)
	}
	if _, err := random.RandomDate(nil, hi, lo); !errors.Is(err, random.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange: %v", err)
	}

	// Ranges beyond time.Duration (~292 years) must not overflow.
	far0 := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	far1 := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	for range 1000 {
		got, err := random.RandomTime(nil, far0, far1)
		if err != nil || got.Before(far0) || got.After(far1) {
			t.Fatalf("RandomTime long = %v, %v", got, err)
		}
		d, err := random.RandomDate(nil, far0, far1)
		if err != nil || d.Before(far0) || d.After(far1) {
			t.Fatalf("RandomDate long = %v, %v", d, err)
		}
	}
}

func TestRandIPs(t *testing.T) {
	if ip := random.RandIPv4(nil); !ip.Is4() {
		t.Fatalf("RandIPv4 = %v", ip)
	}
	if ip := random.RandIPv6(nil); !ip.Is6() {
		t.Fatalf("RandIPv6 = %v", ip)
	}
	s, err := random.RandIP(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := netip.ParseAddr(s); err != nil {
		t.Fatalf("RandIP = %q: %v", s, err)
	}
	for _, p := range []string{"10.0.0.0/8", "192.168.1.0/30", "2001:db8::/32", "1.2.3.4/32", "0.0.0.0/0"} {
		prefix := netip.MustParsePrefix(p)
		for range 100 {
			ip, err := random.RandAddrInPrefix(nil, prefix)
			if err != nil || !prefix.Contains(ip) {
				t.Fatalf("RandAddrInPrefix(%s) = %v, %v", p, ip, err)
			}
		}
	}
	if _, err := random.RandAddrInPrefix(nil, netip.Prefix{}); err == nil {
		t.Fatal("expected error for invalid prefix")
	}
}

func TestDeterministicWithSeed(t *testing.T) {
	a, _ := random.RandString(seeded(), 32, nil)
	b, _ := random.RandString(seeded(), 32, nil)
	if a != b {
		t.Fatalf("same seed, different output: %q vs %q", a, b)
	}
}

// TestConcurrentDefaultSource fails under -race if nil-source helpers share
// unsynchronized state (the previous version did).
func TestConcurrentDefaultSource(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 2000 {
				_, _ = random.RandInt(nil, 0, 100)
				_, _ = random.RandString(nil, 8, nil)
				_, _ = random.Pick(nil, 1, 2, 3)
				_ = random.RandIPv4(nil)
				_, _ = random.SecureInt(0, 10)
				_, _ = random.Pick(random.Secure(), 1, 2)
			}
		})
	}
	wg.Wait()
}
