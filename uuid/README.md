# uuid

[← go-core](../README.md) · [Русская версия](READMEru.md)

RFC 9562 UUIDs for Go: a single comparable `UUID` value type with fast parsing and
formatting, JSON/text/binary/SQL support, and lock-free generators for version 4
(random) and version 7 (time-ordered, strictly monotonic).

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/uuid"
```

## Features

- `NewV7()` — time-ordered, **strictly increasing** UUIDs, ideal for database primary keys
  (RFC 9562 §6.2 method 1: 12-bit counter, overflow carries into the timestamp, safe when
  the clock steps backwards)
- `NewV4()` — random UUIDs from `crypto/rand`, zero allocations
- `Parse` accepts canonical, `{braced}`, `urn:uuid:` and 32-hex forms, case-insensitive, zero allocations
- Implements `encoding.TextMarshaler` / `TextAppender` / `BinaryMarshaler`, `sql.Scanner`,
  `driver.Valuer`; `NullUUID` for nullable columns
- `Version()`, `Variant()`, `Time()` (for v7), `Compare`, `IsZero`, `Nil`, `Max`
- `Generator` with pluggable entropy (`WithReader`, `WithFastRandom`) and clock (`WithClock`)
- The [`db`](db/README.md), [`network`](network/README.md) subpackages return the same type;
  [`google_uuid`](google_uuid/README.md) offers string-ID generators (google/uuid, ULID)

## API

| Function / method | Description |
|---|---|
| `NewV4() UUID` | Random UUID, crypto/rand |
| `NewV7() UUID` | Time-ordered, monotonic UUID, crypto/rand |
| `Parse(s) (UUID, error)` / `MustParse(s)` | Decode text (`ErrInvalidFormat`, `ErrInvalidLength`) |
| `FromBytes(b) (UUID, error)` | From 16 raw bytes |
| `Must(u, err) UUID` | Panic on error, for initialization |
| `u.String()`, `u.URN()`, `u.Bytes()` | Encodings |
| `u.Version()`, `u.Variant()`, `u.Time()` | Inspection |
| `u.Compare(v)`, `u.IsZero()` | Comparison (`==` also works) |
| `MarshalText`/`UnmarshalText`, `MarshalBinary`/`UnmarshalBinary`, `AppendText` | encoding interfaces |
| `Scan`, `Value`; `NullUUID` | database/sql |
| `NewGenerator(opts...)`, `g.NewV4()`, `g.NewV7()` | Custom generators |
| `WithReader(io.Reader)`, `WithFastRandom()`, `WithClock(func() time.Time)` | Generator options |

## Examples

```go
id := uuid.NewV7()
fmt.Println(id)                 // 0192f4c1-9b7a-7d3e-8a41-2f6c0b9e5d17
ts, _ := id.Time()              // creation time, ms precision

u, err := uuid.Parse("{0190A3C2-7B1E-7C3A-9F2D-4B6E8A1C0D3F}")
if err != nil { /* errors.Is(err, uuid.ErrInvalidFormat) */ }

// JSON: encoded as a string
type User struct {
	ID uuid.UUID `json:"id"`
}

// SQL: works as a parameter and a scan target
var userID uuid.UUID
_ = db.QueryRowContext(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)

// Deterministic tests
gen := uuid.NewGenerator(uuid.WithClock(func() time.Time { return fixed }))
a, _ := gen.NewV7()
```

## Concurrency and performance

All functions and `Generator` methods are safe for concurrent use. The v7 state is one
atomic word updated with CAS, so there are no locks. Measured on Intel Core Ultra 5 225H
(14 threads), Go 1.27:

| Benchmark | ns/op | allocs |
|---|---|---|
| `NewV4` | 44 | 0 |
| `NewV7` | 92 | 0 |
| `NewV4` parallel | 64–70 | 0 |
| `NewV7` parallel | ~320 | 0 |
| `String` | 51 | 1 (the string) |
| `Parse` | 25 | 0 |

Parallel `NewV7` is bounded by contention on the single monotonic counter — that is the
cost of a global ordering guarantee. If you need tens of millions of IDs per second, give
each worker its own `Generator` (ordered per worker) or use `network.NewV4`.

## Security

`NewV4`, `NewV7` and default generators use `crypto/rand`: the 62 random bits of v7 and
122 bits of v4 are unpredictable. Remember that v7 exposes its creation time. `WithFastRandom`
and the `network` package are **not** suitable for secrets or tokens.

## Migration

| Before | Now |
|---|---|
| `uuid.NewV4() (UUID, error)` | `uuid.NewV4() UUID` (crypto/rand cannot fail since Go 1.24) |
| `uuid.NewV7(r *rand.ChaCha8) (UUID, error)` | `uuid.NewV7() UUID`; custom entropy: `uuid.NewGenerator(uuid.WithReader(r)).NewV7()` |
| `db.UUID`, `network.UUID`, `uuid.UUID` were three distinct types | `db.UUID` and `network.UUID` are aliases of `uuid.UUID` |
| no parsing/formatting | `Parse`, `String`, text/binary/SQL interfaces |
| v7 not monotonic, 2 random bytes were wasted | monotonic per RFC 9562, 12-bit counter + 62 random bits |
