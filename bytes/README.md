# bytes

[Русская версия](READMEru.md) · [← go-core](../README.md)

Conversion between JSON and Go values: generic `Marshal`/`Unmarshal` with
decoding options, and helpers for free-form `map[string]any` documents that
can keep large integers exact.

```bash
go get github.com/jwm1rr0rb10/go-core
```

The package name shadows the standard library, so import it with an alias:

```go
import corebytes "github.com/jwm1rr0rb10/go-core/bytes"
```

## API

| Item | Description |
|------|-------------|
| `Marshal[T](v) ([]byte, error)` | `json.Marshal` with a typed parameter |
| `Unmarshal[T](data, opts...) (T, error)` | Decode into a new `T` |
| `MapToJSON(m) ([]byte, error)` | Encode a map; `nil` → `null` |
| `JSONToMap(data, opts...) (map[string]any, error)` | Decode an object; `null` → `nil` map |
| `UseNumber()` | Numbers in `any` become `json.Number` instead of `float64` |
| `DisallowUnknownFields()` | Fail on keys that match no struct field |
| `ErrTrailingData` | Returned when the input has more than one JSON value |
| `MapToByteArr`, `ByteArrToMap` | **Deprecated** names of `MapToJSON`, `JSONToMap` |

## Usage

```go
type Event struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
}

ev, err := corebytes.Unmarshal[Event](body, corebytes.DisallowUnknownFields())

// Large IDs survive decoding into a map.
m, err := corebytes.JSONToMap([]byte(`{"order_id": 9007199254740993}`), corebytes.UseNumber())
id, _ := m["order_id"].(json.Number).Int64() // 9007199254740993, not ...992
```

## Notes

- Without options, decoding is exactly `json.Unmarshal`. With options a
  `json.Decoder` is used, but trailing data after the value is still rejected,
  so `{"a":1} garbage` fails the same way in both modes.
- Benchmarks for a small document: `JSONToMap` ≈1.7 µs / 19 allocs,
  with `UseNumber` ≈2.3 µs / 27 allocs.

## Migration

- `ByteArrToMap(string)` → `JSONToMap([]byte, ...DecodeOption)`.
- `MapToByteArr` → `MapToJSON`.
- The old names still work and are marked deprecated.
