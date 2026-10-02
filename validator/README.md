# validator

[Русская версия](READMEru.md) · [← go-core](../README.md)

Input validation with one error type for two styles: struct tags (an `Engine`
on top of [go-playground/validator](https://github.com/go-playground/validator))
and small composable checks for single values. Every failure is a
`ValidationError` mapping field names to messages, ready to be returned to an
API client.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/validator"
```

## Features

- `Engine` is safe for concurrent use and caches struct metadata — create one at
  startup and share it. No global state is required.
- Built-in `date` tag with a configurable layout; custom tags via
  `WithValidation`.
- `WithJSONFieldNames()` reports `email` instead of `Email`; nested fields are
  reported by path (`address.city`).
- `ChainValidator` stops at the first error; `CollectAll` reports every invalid
  field at once.
- Value validators: `EmailValidator` (plain address only, no display names),
  `PhoneValidator` (allocation-free), `UUIDValidator` (canonical form only),
  `TimeValidator`.
- Deterministic error text: fields sorted by name.
- Non-validation errors (for example a non-struct passed to `Struct`) are
  returned unchanged, so bad input and programming mistakes are distinguishable.

## API

| Item | Description |
|------|-------------|
| `NewEngine(opts...) (*Engine, error)` | Create an engine |
| `WithDateLayout(layout)` | Layout for the `date` tag (default `time.DateOnly`) |
| `WithJSONFieldNames()` | Use json tag names in errors |
| `WithValidation(tag, fn)` | Register a custom tag |
| `(*Engine).Struct(s) error` | Validate a struct |
| `(*Engine).Var(field, value, tag) error` | Validate a single value with a tag expression |
| `(*Engine).StructValidator(s) Validator` | Wrap `Struct` as a `Validator` |
| `Default()` / `SetDefault(e)` | Package-level engine (created lazily) |
| `New(dateLayout) error` | Reconfigure the package-level engine |
| `StructValidator(s) Validator` | Validate with the package-level engine |
| `EmailValidator`, `PhoneValidator`, `UUIDValidator(field, value)` | Value checks |
| `NewTimeValidator(field, value, layout)` | Parse check; `Value()` / `Parsed()` return the time |
| `ChainValidator(vs...)` / `CollectAll(vs...)` | First error / all errors merged |
| `ValidatorFunc` | Adapt a `func() error` |
| `ValidationError`, `ErrorFields` | Error type: `Fields map[string]string` |
| `AsValidationError(err)` | Extract a `ValidationError` (value or pointer) from a chain |

## Usage

```go
type CreateUser struct {
	Email    string  `json:"email" validate:"required,email"`
	Birthday string  `json:"birthday" validate:"required,date"`
	Address  Address `json:"address"`
}

var engine, _ = validator.NewEngine(validator.WithJSONFieldNames())

func handle(w http.ResponseWriter, req CreateUser) {
	if err := engine.Struct(req); err != nil {
		if vErr, ok := validator.AsValidationError(err); ok {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(vErr.Fields) // {"email": "...", "address.city": "..."}
			return
		}
		panic(err) // not a struct: programming error
	}
}
```

Composable checks:

```go
err := validator.CollectAll(
	validator.EmailValidator("email", in.Email),
	validator.PhoneValidator("phone", in.Phone),
	validator.UUIDValidator("id", in.ID),
).Validate()
// validation failed: email: must be a valid email address; phone: must be a valid phone number
```

## Performance

Measured on the test machine:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `Engine.Struct`, valid struct (4 fields, nested) | 771 | 97 | 5 |
| `Engine.Struct`, 4 failed fields | 1918 | 1657 | 32 |
| `EmailValidator` | 230 | 96 | 5 |
| `PhoneValidator` | 22 | 0 | 0 (previously regexp + builder: 188 ns, 2 allocs) |

## Migration

- `StructValidator` no longer panics when `New` was not called: a default
  engine with the `date` tag (`2006-01-02`) is created lazily. `New` can now be
  called again to change the layout (previously only the first call counted).
- Engines are created with `WithRequiredStructEnabled`, so `required` on a
  struct-typed field now fails for the zero struct (this is the go-playground
  v11 default).
- Nested fields are reported by path (`Address.City`) instead of the bare leaf
  name (`City`). Top-level field names are unchanged.
- Messages include the tag parameter when there is one, e.g.
  `failed on the 'gte' tag (18)`.
- `ValidationError.Error()` now reads `validation failed: a: msg; b: msg`
  instead of `map[a:msg b:msg]`.
- `EmailValidator` rejects `Name <a@b.c>`, surrounding spaces, comments and
  addresses longer than 254 characters.
- `UUIDValidator` accepts only the canonical 36-character form (no braces,
  `urn:uuid:` prefix or 32-digit form).
- `TimeValidator.Value()` parses lazily if `Validate` was not called; `Parsed()`
  reports validity.
- `ChainValidator` skips `nil` validators instead of panicking.
