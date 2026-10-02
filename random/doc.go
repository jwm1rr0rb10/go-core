// Package random generates random numbers, strings, dates, IP addresses and
// picks, with two families of functions:
//
//   - Fast, non-cryptographic helpers (RandInt, RandString, Pick, ...). They
//     take an optional *rand.Rand; pass nil to use the math/rand/v2 global
//     generator, which is goroutine-safe and lock-free. A non-nil *rand.Rand
//     is NOT goroutine-safe: give each goroutine its own, or guard it.
//   - Cryptographically secure helpers (SecureInt, SecureString, SecureToken,
//     ...) backed by crypto/rand, for tokens, passwords, nonces and anything an
//     attacker must not predict.
//
// All range functions are half-open [min, max) unless documented otherwise,
// and handle the full numeric range without overflow.
package random
