# safe

[Русская версия](READMEru.md) · [← go-core](../README.md)

Turns panics into errors. A panic in a goroutine you started takes the whole
process down. `safe` recovers it, reports it to a handler, and gives it back
to you as a typed `*PanicError` with the panic value and the stack captured
at the panic site.

```go
import "github.com/jwm1rr0rb10/go-core/safe"
```

Subpackages:

- [`safe/errorgroup`](errorgroup/README.md): panic-safe `errgroup` with error
  collection.
- [`safe/waitgroup`](waitgroup/README.md): a `sync.WaitGroup` that rejects
  misuse and can wait with a timeout.

## Features

- `*PanicError{Value, Stack}` works with `errors.As`. `panic(err)` unwraps
  to `err`, so `errors.Is` works too.
- The stack is captured **once**, inside the recovering frame, so it shows
  the panicking function.
- Pluggable `RecoverFunc`:
  - `DefaultRecover` logs via `slog.Default()`;
  - `IgnoreRecover` stays silent.
- The no-panic path costs ~7 ns and makes zero allocations.

## API

| Function | Purpose |
|---|---|
| `Go(ctx, fn, recoverFn) <-chan error` | Run `fn` in a goroutine; the channel gets its error or `*PanicError` |
| `Call(fn, recoverFn) error` | Run `func() error` inline, recovering panics |
| `CallCtx(ctx, fn, recoverFn) error` | Same for `func(context.Context) error` |
| `Recover(&err, recoverFn)` | `defer` it in your own function to convert a panic into `err` |
| `Func(fn, recoverFn)`, `CtxFunc(fn, recoverFn)` | Wrap a function so calling it never panics |
| `NewPanicError(v)` | Build a `*PanicError` from a recovered value |
| `DefaultRecover`, `IgnoreRecover` | Ready-made `RecoverFunc`s (`nil` means `DefaultRecover`) |

## Usage

```go
// Background worker that must not crash the service.
errc := safe.Go(ctx, func(ctx context.Context) error {
	return consumer.Run(ctx)
}, nil) // nil: log panics with slog.Default()

if err := <-errc; err != nil {
	var pe *safe.PanicError
	if errors.As(err, &pe) {
		metrics.Panics.Inc()
	}
}

// Inside your own function.
func (h *Handler) process(msg Message) (err error) {
	defer safe.Recover(&err, func(p *safe.PanicError) {
		h.log.Error("panic", "value", p.Value, "stack", string(p.Stack))
	})
	return h.handle(msg)
}
```

## Performance

Intel Core Ultra 5 225H, Go 1.27:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `Call`, no panic | 6.6 | 0 | 0 |
| `Call`, panic (stack capture) | 10 100 | 3120 | 3 |
| `Go` + receive | 446 | 176 | 3 |

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| `RecoverFunc func(r any)` | `RecoverFunc func(p *PanicError)`: the handler gets the value **and** the stack |
| Panic error was `fmt.Errorf("panic: %v\n<stack>")`, a string you could not inspect | `*PanicError`. `Error()` is `"panic: <value>"`; the stack is in the `Stack` field |
| Stack captured twice (in the handler and in the error) | Captured once |
| `SafeGo`, `SafeFunc`, `SafeCtxFunc` | Renamed to `Go`, `Func`, `CtxFunc`; old names remain as deprecated aliases |
| none | `Call`, `CallCtx`, `Recover`, `IgnoreRecover`, `NewPanicError` |
