package random

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"strings"
)

// cryptoSource is a rand.Source backed by crypto/rand. It holds no state, so
// it is safe for concurrent use.
type cryptoSource struct{}

func (cryptoSource) Uint64() uint64 {
	var b [8]byte
	_, _ = cryptorand.Read(b[:]) // cannot fail since Go 1.24
	return binary.LittleEndian.Uint64(b[:])
}

// secure is safe for concurrent use: rand.Rand keeps no state of its own and
// cryptoSource is stateless.
var secure = rand.New(cryptoSource{})

// Secure returns a *rand.Rand backed by crypto/rand. Unlike other *rand.Rand
// values it is safe for concurrent use. It can be passed to every function in
// this package that takes a *rand.Rand, e.g. Pick(random.Secure(), ...).
func Secure() *rand.Rand { return secure }

// SecureInt returns a cryptographically secure uniform int in [min, max).
func SecureInt(min, max int) (int, error) { return RandInt(secure, min, max) }

// SecureInt64 returns a cryptographically secure uniform int64 in [min, max).
func SecureInt64(min, max int64) (int64, error) { return RandInt64(secure, min, max) }

// SecureString returns a cryptographically secure random string of n bytes
// drawn uniformly (no modulo bias) from set, or DefaultSet when set is empty.
// It reads entropy in bulk, so it is much faster than RandString(Secure(), ...).
func SecureString(n int, set []byte) (string, error) {
	if n < 0 {
		return "", ErrNegativeCount
	}
	if len(set) == 0 {
		set = DefaultSet
	}
	if len(set) > 256 {
		return RandString(secure, n, set)
	}
	// Rejection sampling on bytes: accept b < limit, where limit is the
	// largest multiple of len(set) not above 256.
	m := len(set)
	limit := 256 - 256%m
	var out strings.Builder
	out.Grow(n)
	var buf [64]byte
	for out.Len() < n {
		_, _ = cryptorand.Read(buf[:])
		for _, b := range buf {
			if int(b) < limit {
				out.WriteByte(set[int(b)%m])
				if out.Len() == n {
					break
				}
			}
		}
	}
	return out.String(), nil
}

// SecureToken returns n cryptographically secure random bytes encoded as
// 2n lowercase hex characters, e.g. for API keys or CSRF tokens.
func SecureToken(n int) (string, error) {
	if n < 0 {
		return "", ErrNegativeCount
	}
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b), nil
}

// SecureText returns a 26-character base32 string with 128 bits of entropy
// (crypto/rand.Text). It is a good default for session IDs and secrets.
func SecureText() string { return cryptorand.Text() }
