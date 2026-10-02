# uuid/network

[← go-core](../../README.md) · [← uuid](../README.md) · [Русская версия](READMEru.md)

Fast UUIDs for identifiers that do not need to be unpredictable: request IDs, trace and
span IDs, message and correlation IDs. Randomness comes from the `math/rand/v2` global
generator (per-thread ChaCha8, seeded by the runtime), which is goroutine-safe and lock-free.

```go
import "github.com/jwm1rr0rb10/go-core/uuid/network"
```

## API

| Function | Description |
|---|---|
| `NewV4() UUID` | Random v4 from the fast PRNG |
| `NewV7() UUID` | Time-ordered, strictly monotonic v7 from the fast PRNG |
| `type UUID = uuid.UUID` | Same type as the root package |

## Example

```go
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := network.NewV4()
		w.Header().Set("X-Request-ID", id.String())
		next.ServeHTTP(w, r)
	})
}
```

## Performance

Intel Core Ultra 5 225H (14 threads), Go 1.27, 0 allocations:

| Benchmark | ns/op |
|---|---|
| `NewV4` | 23 |
| `NewV4` parallel | **1.3** |
| `NewV7` | 80–96 |
| `NewV7` parallel | ~260 (shared monotonic counter) |

`NewV4` scales linearly with cores because there is no shared state at all.

## Security

The output of `math/rand/v2` is not cryptographically guaranteed to be unpredictable. Do
not use these IDs as session tokens, password-reset links or anything an attacker must not
guess; use the root `uuid` package instead.

## Migration

- **Data race fixed**: the previous version shared one `*rand.ChaCha8` across goroutines
  without synchronization, which could produce duplicate IDs under load.
- `init()` no longer panics; there is no package initialization at all.
- `network.UUID` is now an alias of `uuid.UUID`.
- `NewV7` is now strictly monotonic.
