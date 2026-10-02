package random

import (
	"math"
	"math/rand/v2"
	"net/netip"
	"strings"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

// Errors returned for invalid arguments. Match them with errors.Is.
var (
	ErrInvalidRange  = errors.New("random: invalid range")
	ErrNegativeCount = errors.New("random: n must be non-negative")
	ErrEmpty         = errors.New("random: no values to choose from")
)

// DefaultSet is the alphabet used by RandString and SecureString when set is
// empty: A-Z, a-z, 0-9.
var DefaultSet = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

// uint64n returns a uniform value in [0, n) from r, or from the global
// generator when r is nil. n must be > 0.
func uint64n(r *rand.Rand, n uint64) uint64 {
	if r == nil {
		return rand.Uint64N(n)
	}
	return r.Uint64N(n)
}

func float64v(r *rand.Rand) float64 {
	if r == nil {
		return rand.Float64()
	}
	return r.Float64()
}

func uint64v(r *rand.Rand) uint64 {
	if r == nil {
		return rand.Uint64()
	}
	return r.Uint64()
}

// RandInt returns a uniform random int in [min, max). r may be nil.
// The full int range is supported: RandInt(nil, math.MinInt, math.MaxInt).
func RandInt(r *rand.Rand, min, max int) (int, error) {
	n, err := RandInt64(r, int64(min), int64(max))
	return int(n), err
}

// RandInt64 returns a uniform random int64 in [min, max). r may be nil.
func RandInt64(r *rand.Rand, min, max int64) (int64, error) {
	if max <= min {
		return 0, ErrInvalidRange
	}
	span := uint64(max) - uint64(min) // exact even when max-min overflows int64
	return int64(uint64(min) + uint64n(r, span)), nil
}

// RandFloat64 returns a random float64 in [min, max). Both bounds must be
// finite. r may be nil.
func RandFloat64(r *rand.Rand, min, max float64) (float64, error) {
	if !(max > min) || math.IsInf(min, 0) || math.IsInf(max, 0) {
		return 0, ErrInvalidRange
	}
	v := min + float64v(r)*(max-min)
	if v >= max { // rounding at the top of the range
		v = math.Nextafter(max, min)
	}
	return v, nil
}

// RandomBool returns true or false with equal probability. r may be nil.
func RandomBool(r *rand.Rand) bool { return uint64v(r)&1 == 1 }

// RandString returns a random string of n bytes drawn uniformly from set
// (DefaultSet when set is empty). r may be nil. The result is built with a
// single allocation, and for sets of up to 256 bytes each 64-bit random draw
// yields up to 8 characters (unbiased rejection sampling).
func RandString(r *rand.Rand, n int, set []byte) (string, error) {
	if n < 0 {
		return "", ErrNegativeCount
	}
	if len(set) == 0 {
		set = DefaultSet
	}
	var b strings.Builder
	b.Grow(n)
	m := len(set)
	if m > 256 {
		for range n {
			b.WriteByte(set[uint64n(r, uint64(m))])
		}
		return b.String(), nil
	}
	limit := 256 - 256%m // accept bytes below the largest multiple of m
	for b.Len() < n {
		x := uint64v(r)
		for i := 0; i < 8 && b.Len() < n; i++ {
			if v := int(byte(x)); v < limit {
				b.WriteByte(set[v%m])
			}
			x >>= 8
		}
	}
	return b.String(), nil
}

// Pick returns a uniformly chosen element of values. r may be nil.
func Pick[T any](r *rand.Rand, values ...T) (T, error) {
	if len(values) == 0 {
		var zero T
		return zero, ErrEmpty
	}
	return values[uint64n(r, uint64(len(values)))], nil
}

// RandomCase returns a uniformly chosen argument.
//
// Deprecated: use the type-safe Pick.
func RandomCase(r *rand.Rand, args ...any) (any, error) { return Pick(r, args...) }

// RandomTime returns a uniform random instant in [min, max] (inclusive) with
// nanosecond precision. Ranges longer than ~292 years are supported. The
// result carries min's location. r may be nil.
func RandomTime(r *rand.Rand, min, max time.Time) (time.Time, error) {
	if max.Before(min) {
		return time.Time{}, ErrInvalidRange
	}
	if d := max.Sub(min); d < math.MaxInt64 { // not saturated
		return min.Add(time.Duration(uint64n(r, uint64(d)+1))), nil
	}
	// Very long range: choose whole seconds, then nanoseconds, then clamp.
	secs := uint64(max.Unix() - min.Unix())
	t := time.Unix(min.Unix()+int64(uint64n(r, secs+1)), int64(uint64n(r, 1e9))).In(min.Location())
	if t.Before(min) {
		return min, nil
	}
	if t.After(max) {
		return max, nil
	}
	return t, nil
}

// RandomDate returns a random instant in [min, max] (inclusive) with
// whole-second offsets from min, as in previous versions. Use RandomTime for
// nanosecond precision. r may be nil.
func RandomDate(r *rand.Rand, min, max time.Time) (time.Time, error) {
	if max.Before(min) {
		return time.Time{}, ErrInvalidRange
	}
	secs := uint64(max.Sub(min) / time.Second)
	if max.Sub(min) == math.MaxInt64 { // saturated: fall back to Unix seconds
		secs = uint64(max.Unix() - min.Unix())
	}
	t := min.Add(time.Duration(uint64n(r, secs+1)) * time.Second)
	if t.After(max) || t.Before(min) {
		return max, nil
	}
	return t, nil
}

// RandIPv4 returns a uniformly random IPv4 address (any of 2^32, including
// reserved ranges). r may be nil.
func RandIPv4(r *rand.Rand) netip.Addr {
	v := uint32(uint64v(r))
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// RandIPv6 returns a uniformly random IPv6 address. r may be nil.
func RandIPv6(r *rand.Rand) netip.Addr {
	var b [16]byte
	hi, lo := uint64v(r), uint64v(r)
	for i := range 8 {
		b[i] = byte(hi >> (56 - 8*i))
		b[8+i] = byte(lo >> (56 - 8*i))
	}
	return netip.AddrFrom16(b)
}

// RandAddrInPrefix returns a random address inside prefix, for example
// 10.0.0.0/8 or 2001:db8::/32. r may be nil.
func RandAddrInPrefix(r *rand.Rand, prefix netip.Prefix) (netip.Addr, error) {
	if !prefix.IsValid() {
		return netip.Addr{}, ErrInvalidRange
	}
	prefix = prefix.Masked()
	base := prefix.Addr().AsSlice()
	hostBits := len(base)*8 - prefix.Bits()
	for i := len(base) - 1; i >= 0 && hostBits > 0; i-- {
		bits := min(hostBits, 8)
		mask := byte(1<<bits - 1)
		base[i] |= byte(uint64v(r)) & mask
		hostBits -= bits
	}
	addr, _ := netip.AddrFromSlice(base)
	return addr, nil
}

// RandIP returns a random IPv4 address in dotted form. The error is always nil.
//
// Deprecated: use RandIPv4, which returns a netip.Addr.
func RandIP(r *rand.Rand) (string, error) { return RandIPv4(r).String(), nil }
