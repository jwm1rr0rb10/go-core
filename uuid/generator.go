package uuid

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	mathrand "math/rand/v2"
	"sync/atomic"
	"time"
)

// Generator creates UUIDs from a configurable entropy source and clock.
//
// A Generator is safe for concurrent use. Version 7 UUIDs from one Generator
// are strictly increasing: the 12-bit rand_a field is a counter that starts at
// a random value in each new millisecond and, on overflow, carries into the
// timestamp (RFC 9562, section 6.2, method 1). The state is a single atomic
// word, so generation is lock-free.
//
// The zero value is not usable; create generators with NewGenerator.
type Generator struct {
	state  atomic.Uint64 // unix_ms<<12 | counter of the last v7 UUID
	source source
	reader io.Reader // used when source == sourceReader
	now    func() time.Time
}

type source uint8

const (
	sourceCrypto source = iota // crypto/rand
	sourceFast                 // math/rand/v2 globals
	sourceReader               // user-supplied io.Reader
)

// Option configures a Generator.
type Option func(*Generator)

// WithReader sets the entropy source. r must be safe for concurrent use if
// the Generator is shared between goroutines. The default is crypto/rand.
func WithReader(r io.Reader) Option {
	return func(g *Generator) {
		if r != nil {
			g.source, g.reader = sourceReader, r
		}
	}
}

// WithFastRandom makes the Generator use the math/rand/v2 global generator
// instead of crypto/rand. It is several times faster and lock-free, but the
// output is predictable to an attacker: use it only for non-secret IDs.
func WithFastRandom() Option {
	return func(g *Generator) { g.source, g.reader = sourceFast, nil }
}

// WithClock sets the time source used for version 7 timestamps. It is mainly
// useful in tests. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(g *Generator) {
		if now != nil {
			g.now = now
		}
	}
}

// NewGenerator returns a Generator configured by opts.
func NewGenerator(opts ...Option) *Generator {
	g := &Generator{now: time.Now}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// read fills b from the configured source. crypto/rand.Read cannot fail
// since Go 1.24 (it crashes the program instead), so only a custom reader
// can return an error.
func (g *Generator) read(b []byte) error {
	switch g.source {
	case sourceFast:
		for len(b) >= 8 {
			binary.LittleEndian.PutUint64(b, mathrand.Uint64())
			b = b[8:]
		}
		if len(b) > 0 {
			var tail [8]byte
			binary.LittleEndian.PutUint64(tail[:], mathrand.Uint64())
			copy(b, tail[:])
		}
		return nil
	case sourceReader:
		return g.readCustom(b)
	default:
		_, _ = cryptorand.Read(b)
		return nil
	}
}

// readCustom reads through an intermediate buffer so that b, usually a stack
// array in the caller, does not escape to the heap via the io.Reader call.
func (g *Generator) readCustom(b []byte) error {
	tmp := make([]byte, len(b))
	if _, err := io.ReadFull(g.reader, tmp); err != nil {
		return err
	}
	copy(b, tmp)
	return nil
}

var defaultGenerator = NewGenerator()

// NewV4 returns a random (version 4) UUID using crypto/rand.
func NewV4() UUID {
	var u UUID
	_, _ = cryptorand.Read(u[:])
	setV4(&u)
	return u
}

// NewV7 returns a time-ordered (version 7) UUID using crypto/rand and the
// package default Generator. Successive calls return strictly increasing
// values.
func NewV7() UUID {
	u, _ := defaultGenerator.NewV7() // the default reader cannot fail
	return u
}

// NewV4 returns a random (version 4) UUID.
func (g *Generator) NewV4() (UUID, error) {
	var u UUID
	if err := g.read(u[:]); err != nil {
		return Nil, err
	}
	setV4(&u)
	return u, nil
}

// NewV7 returns a time-ordered (version 7) UUID that is greater than every
// UUID previously returned by g.NewV7.
func (g *Generator) NewV7() (UUID, error) {
	var rnd [10]byte // 2 bytes counter seed + 8 bytes rand_b
	if err := g.read(rnd[:]); err != nil {
		return Nil, err
	}
	seed := uint64(binary.BigEndian.Uint16(rnd[:2]) & 0x07ff) // 11 bits: leave room to count
	return makeV7(g.nextState(seed), binary.BigEndian.Uint64(rnd[2:])), nil
}

// nextState advances the v7 state and returns unix_ms<<12 | counter.
func (g *Generator) nextState(seed uint64) uint64 {
	ms := g.now().UnixMilli()
	if ms < 0 {
		ms = 0
	}
	ts := uint64(ms) & (1<<48 - 1)
	for {
		last := g.state.Load()
		next := last + 1 // same (or earlier) millisecond: bump the counter
		if ts > last>>12 {
			next = ts<<12 | seed // new millisecond: random counter start
		}
		if g.state.CompareAndSwap(last, next) {
			return next
		}
	}
}

func makeV7(state, randB uint64) UUID {
	var u UUID
	ms := state >> 12
	u[0] = byte(ms >> 40)
	u[1] = byte(ms >> 32)
	u[2] = byte(ms >> 24)
	u[3] = byte(ms >> 16)
	u[4] = byte(ms >> 8)
	u[5] = byte(ms)
	u[6] = 0x70 | byte(state>>8)&0x0f
	u[7] = byte(state)
	binary.BigEndian.PutUint64(u[8:], randB)
	u[8] = 0x80 | u[8]&0x3f
	return u
}

func setV4(u *UUID) {
	u[6] = 0x40 | u[6]&0x0f
	u[8] = 0x80 | u[8]&0x3f
}
