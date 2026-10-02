# uuid/db

[← go-core](../../README.md) · [← uuid](../README.md) · [Русская версия](READMEru.md)

UUID constructors for database keys. A thin, explicit-intent layer over the root
[`uuid`](../README.md) package: cryptographically random bits and strictly monotonic v7,
which keeps B-tree indexes append-only and avoids page splits.

```go
import "github.com/jwm1rr0rb10/go-core/uuid/db"
```

## API

| Function | Description |
|---|---|
| `NewV7() (UUID, error)` | Time-ordered, monotonic, crypto/rand. Recommended for primary keys |
| `NewV4() (UUID, error)` | Random, crypto/rand |
| `type UUID = uuid.UUID` | Same type as the root package: `Parse`, `String`, JSON, `sql.Scanner`, `driver.Valuer` |

The error is always `nil` (crypto/rand cannot fail since Go 1.24); it is kept for API
compatibility. New code can call `uuid.NewV7()` directly.

## Example

```go
id, _ := db.NewV7()
_, err := pool.Exec(ctx, "INSERT INTO orders (id, total) VALUES ($1, $2)", id, total)

var got db.UUID
err = pool.QueryRow(ctx, "SELECT id FROM orders LIMIT 1").Scan(&got)
```

## Performance

Intel Core Ultra 5 225H, Go 1.27: `NewV7` ≈ 100–135 ns/op serial, ≈ 350 ns/op with 14
goroutines, 0 allocations. Safe for concurrent use; uniqueness and per-goroutine ordering are
tested with 8 × 100 000 IDs under `-race`.

## Migration

- `db.UUID` used to be a separate `[16]byte` type; it is now an alias of `uuid.UUID`.
  Conversions like `uuid.UUID(dbID)` still compile and can be removed.
- `NewV7` is now strictly monotonic (it was random within a millisecond).
