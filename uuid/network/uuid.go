// Package network provides fast UUID constructors for non-security
// identifiers such as request, trace or message IDs.
//
// Randomness comes from the math/rand/v2 global generator (ChaCha8 state per
// runtime thread), which is goroutine-safe, lock-free and seeded by the
// runtime from the OS. It is not suitable for secrets, tokens or anything an
// attacker must not predict; use the root uuid package for those.
//
// UUID is an alias of uuid.UUID.
package network

import (
	"encoding/binary"
	"math/rand/v2"

	"github.com/jwm1rr0rb10/go-core/uuid"
)

// UUID is an alias of uuid.UUID.
type UUID = uuid.UUID

var generator = uuid.NewGenerator(uuid.WithFastRandom())

// NewV4 returns a version 4 UUID built from a fast, non-cryptographic PRNG.
func NewV4() UUID {
	var b [16]byte
	binary.LittleEndian.PutUint64(b[:8], rand.Uint64())
	binary.LittleEndian.PutUint64(b[8:], rand.Uint64())
	b[6] = 0x40 | b[6]&0x0f
	b[8] = 0x80 | b[8]&0x3f
	return UUID(b)
}

// NewV7 returns a strictly monotonic version 7 UUID whose random bits come
// from a fast, non-cryptographic PRNG.
func NewV7() UUID {
	u, _ := generator.NewV7() // the fast source never fails
	return u
}
