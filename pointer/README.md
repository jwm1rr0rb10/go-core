# pointer

[Русская версия](READMEru.md) · [← go-core](../README.md)

Generic helpers for optional values represented as pointers: create them from
literals, read them with fallbacks, compare and copy them. Handy for PATCH
requests, nullable database columns and optional config fields.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/pointer"
```

## API

| Function | Description |
|----------|-------------|
| `ToPointer(v) *T` | Pointer to a copy of `v` (on Go 1.26+ `new(v)` does the same) |
| `ToPointerOrNil(v) *T` | `nil` for the zero value, otherwise a pointer to a copy |
| `FromPointer(p) (T, bool)` | Value and whether `p` was non-nil |
| `Deref(p) T` | Value or the zero value |
| `ValueOr(p, fallback) T` | Value or `fallback` (`FromPointerOr` is the same) |
| `Coalesce(ps...) *T` | First non-nil pointer |
| `Equal(a, b) bool` | Both nil, or both non-nil with equal values |
| `Clone(p) *T` | Shallow copy of `*p` in new memory; `nil` stays `nil` |
| `Set(p, v) bool` | Assign if `p` is non-nil |
| `Swap(a, b)` | Exchange values; panics on nil |
| `Copy(p) (*T, error)` | **Deprecated**, use `Clone` |
| `IsNil(p) bool` | **Deprecated**, write `p == nil` |

## Usage

```go
type UpdateUser struct {
	Name *string `json:"name,omitempty"`
	Age  *int    `json:"age,omitempty"`
}

// Only non-empty form values become fields to update.
req := UpdateUser{
	Name: pointer.ToPointerOrNil(form.Name),
	Age:  pointer.ToPointerOrNil(form.Age),
}

limit := pointer.ValueOr(cfg.Limit, 100)
env := pointer.Coalesce(flagEnv, fileEnv, pointer.ToPointer("dev"))

if !pointer.Equal(old.Name, req.Name) {
	// name changed
}
```

## Notes

- `Clone` copies the value only. Slices, maps and pointers inside it are shared
  with the original.
- No reflection; the only allocations are the new pointers returned by `ToPointer`, `ToPointerOrNil` and `Clone`.

## Migration

- `Copy` no longer uses reflection and no longer rejects non-comparable types
  (slices, maps, structs containing them); its error is always `nil`. Use
  `Clone`, which returns just the pointer.
- `IsNil` is deprecated.
- New: `ToPointerOrNil`, `Deref`, `ValueOr`, `Coalesce`, `Equal`, `Clone`.
