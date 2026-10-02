# random

[← go-core](../README.md) · [Русская версия](READMEru.md)

Random numbers, strings, times, IP addresses and picks — in two clearly separated flavors:
**fast** helpers on top of `math/rand/v2` for tests, sampling, jitter and load generation,
and **secure** helpers on top of `crypto/rand` for tokens, passwords and secrets.

```go
import "github.com/jwm1rr0rb10/go-core/random"
```

## Features

- Goroutine-safe by default: pass `nil` as the source to use the lock-free `math/rand/v2` globals
- Reproducible when needed: pass a seeded `*rand.Rand`
- Ranges handle the full numeric domain (`RandInt64(nil, math.MinInt64, math.MaxInt64)` works)
- Unbiased strings via rejection sampling; one allocation per string; 8 characters per 64-bit draw
- `crypto/rand` family: `SecureInt`, `SecureString`, `SecureToken`, `SecureText`, `Secure()`
- `netip.Addr` helpers: random IPv4/IPv6, random address inside a CIDR prefix
- Generic `Pick[T]`
- Sentinel errors: `ErrInvalidRange`, `ErrNegativeCount`, `ErrEmpty`

## API

| Fast (`r *rand.Rand` may be nil) | Secure (crypto/rand) | Description |
|---|---|---|
| `RandInt(r, min, max)` / `RandInt64` | `SecureInt(min, max)` / `SecureInt64` | Integer in `[min, max)` |
| `RandFloat64(r, min, max)` | — | Float in `[min, max)` |
| `RandString(r, n, set)` | `SecureString(n, set)` | `n` bytes from `set` (default A–Z a–z 0–9) |
| — | `SecureToken(n)` | `n` random bytes as hex |
| — | `SecureText()` | 26-char base32, 128 bits (`crypto/rand.Text`) |
| `Pick(r, values...)` | `Pick(random.Secure(), ...)` | Uniform choice |
| `RandomBool(r)` | — | Coin flip |
| `RandomTime(r, min, max)` | — | Instant in `[min, max]`, ns precision |
| `RandomDate(r, min, max)` | — | Instant in `[min, max]`, whole-second offsets |
| `RandIPv4(r)`, `RandIPv6(r)` | — | Random `netip.Addr` |
| `RandAddrInPrefix(r, prefix)` | — | Address inside e.g. `10.0.0.0/8` |
| — | `Secure() *rand.Rand` | Concurrency-safe crypto-backed `*rand.Rand` for any function above |

Deprecated (kept for compatibility): `RandomCase` → `Pick`, `RandIP` → `RandIPv4`.

## Examples

```go
// Fast, goroutine-safe
n, _ := random.RandInt(nil, 1, 7)
jitter := time.Duration(n) * 10 * time.Millisecond

// Reproducible test data
r := rand.New(rand.NewPCG(42, 42))
name, _ := random.RandString(r, 8, nil)

// Secrets
apiKey, _ := random.SecureToken(32)      // 64 hex chars
password, _ := random.SecureString(20, []byte("abcdefghjkmnpqrstuvwxyz23456789"))
sessionID := random.SecureText()

// Network test data
ip, _ := random.RandAddrInPrefix(nil, netip.MustParsePrefix("192.168.0.0/16"))
```

## Concurrency and performance

Functions called with a `nil` source and all `Secure*` functions are safe for concurrent use.
A non-nil `*rand.Rand` created by you is **not** — give each goroutine its own. `Secure()` is
the exception: it is stateless and safe to share.

Intel Core Ultra 5 225H (14 threads), Go 1.27:

| Benchmark | ns/op | allocs |
|---|---|---|
| `RandInt` | 7.3 | 0 |
| `RandInt` parallel | 0.9 | 0 |
| `RandString(32)` | 124 | 1 |
| `RandString(32)` parallel | 20 | 1 |
| `SecureString(32)` | 229 | 1 |
| `SecureInt` | 48 | 0 |

## Security

Only the `Secure*` functions and `Secure()` are suitable for anything an attacker must not
predict. The fast functions are statistically good but not cryptographically secure.

## Migration

- **Thread-safety bug fixed**: the package-level source was a `rand.New(rand.NewPCG(...))`
  shared across goroutines, which is not safe; `nil` now means the goroutine-safe globals.
- `RandInt`/`RandInt64` no longer panic when `max-min` overflows.
- `RandFloat64` rejects NaN/±Inf bounds.
- Errors are now sentinels (`ErrInvalidRange`, `ErrNegativeCount`, `ErrEmpty`); match with `errors.Is`.
- Output for a given seed differs from the previous version (`RandString` draws 8 characters
  per 64-bit value).
- `RandomCase` and `RandIP` are deprecated in favor of `Pick` and `RandIPv4`.
- New: `RandomTime`, `RandIPv6`, `RandAddrInPrefix`, `Secure*`, `Pick`.
