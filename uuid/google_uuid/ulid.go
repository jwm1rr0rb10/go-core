package google_uuid

import (
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ULIDGenerator generates monotonic ULIDs (26-character Crockford base32).
//
// Entropy comes from ChaCha8 (a CSPRNG) seeded from crypto/rand. IDs created
// by one generator are strictly increasing, even within one millisecond or
// when the wall clock steps backwards. ULIDGenerator is safe
// for concurrent use; calls are serialized by a mutex.
type ULIDGenerator struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
	lastMs  uint64
}

// NewULIDGenerator returns a ULID generator with a fresh random seed.
func NewULIDGenerator() *ULIDGenerator {
	var seed [32]byte
	_, _ = cryptorand.Read(seed[:]) // cannot fail since Go 1.24
	return &ULIDGenerator{entropy: ulid.Monotonic(mathrand.NewChaCha8(seed), 0)}
}

// New returns the next ULID.
func (g *ULIDGenerator) New() ulid.ULID {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := max(ulid.Timestamp(time.Now()), g.lastMs) // never go back in time
	for {
		id, err := ulid.New(ms, g.entropy)
		if err == nil {
			g.lastMs = ms
			return id
		}
		// ulid.ErrMonotonicOverflow: the 80-bit entropy for this millisecond
		// is exhausted. Move to the next millisecond, keeping order.
		ms++
	}
}

// GenerateID returns the next ULID as a string.
func (g *ULIDGenerator) GenerateID() string { return g.New().String() }
