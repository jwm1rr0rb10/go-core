# uuid/google_uuid

[← go-core](../../README.md) · [← uuid](../README.md) · [Русская версия](READMEru.md)

String ID generators behind one interface, `IDGenerator`: UUID v4 and v7 backed by
[google/uuid](https://github.com/google/uuid) and monotonic
[ULID](https://github.com/ulid/spec)s backed by [oklog/ulid](https://github.com/oklog/ulid).
Use it when a component should depend on "something that makes string IDs" and the concrete
format is chosen at wiring time.

The directory name contains an underscore for historical reasons; import it with an alias:

```go
import idgen "github.com/jwm1rr0rb10/go-core/uuid/google_uuid"
```

## API

| Identifier | Description |
|---|---|
| `type IDGenerator interface { GenerateID() string }` | Common contract |
| `NewGoogleUUIDGenerator()` | UUIDv4 strings |
| `NewGoogleUUIDv7Generator()` | UUIDv7 strings |
| `NewULIDGenerator()` | Monotonic ULIDs, 26 chars, lexicographically sortable |
| `(*ULIDGenerator).New() ulid.ULID` | Typed ULID |

All generators are safe for concurrent use and never fail.

## Example

```go
type OrderService struct{ ids idgen.IDGenerator }

svc := OrderService{ids: idgen.NewULIDGenerator()}
id := svc.ids.GenerateID() // "01J9Z3K8V6Q2N4R7T1W5X8Y0C3"
```

## ULID details

Entropy is ChaCha8 (a CSPRNG) seeded from `crypto/rand`, wrapped in `ulid.Monotonic`. IDs
from one generator are strictly increasing, even within one millisecond or when the wall
clock steps backwards; if the 80-bit entropy for a millisecond is exhausted, the timestamp
moves forward by 1 ms. Calls are serialized by a mutex.

## Performance

Intel Core Ultra 5 225H, Go 1.27:

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| ULID | 128 | 48 | 2 |
| ULID parallel (14 goroutines) | 326 | 48 | 2 |
| google UUIDv4 | 129 | 64 | 2 |
| google UUIDv7 | 178 | 64 | 2 |

If you need typed values or zero allocations, use the root [`uuid`](../README.md) package.

## Migration

- **Data race fixed** in `ULIDGenerator`: the ChaCha8 state was used from several goroutines
  without a lock.
- `NewULIDGenerator()` now returns `*ULIDGenerator` (no error).
- `(*ULIDGenerator).GenerateID()` now returns `string` (no error), so `ULIDGenerator`
  finally implements `IDGenerator`.
- ULIDs are now monotonic.
- New: `NewGoogleUUIDv7Generator`, `(*ULIDGenerator).New`.
