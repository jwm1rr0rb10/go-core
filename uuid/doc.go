// Package uuid implements RFC 9562 UUIDs: a single value type with parsing,
// text/binary/SQL encoding, and generators for versions 4 and 7.
//
// The package-level constructors use crypto/rand and are safe for concurrent
// use:
//
//	id := uuid.NewV7() // time-ordered, monotonic, good for primary keys
//	rnd := uuid.NewV4() // fully random
//
// UUIDv7 values produced by one Generator (including the package default) are
// strictly increasing, even when many are created within the same
// millisecond or the wall clock steps backwards (RFC 9562, section 6.2,
// method 1: a 12-bit counter in rand_a).
//
// The subpackages db and network return the same UUID type; network trades
// cryptographic randomness for speed.
package uuid
