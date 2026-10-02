# safe/errorgroup

[Русская версия](READMEru.md) · [← safe](../README.md) · [← go-core](../../README.md)

A drop-in alternative to `golang.org/x/sync/errgroup`. A panic in one task
becomes an error instead of killing the process, and you choose whether to
get the first error or all of them.

```go
import "github.com/jwm1rr0rb10/go-core/safe/errorgroup"
```

## Features

- Same shape as `errgroup`: `Go`, `TryGo`, `SetLimit`, `Wait`, `WithContext`,
  and a usable zero value.
- **Panics** are recovered once, passed to a `safe.RecoverFunc`, and recorded
  as `*safe.PanicError`.
- **First error** (default) or **all errors** joined with `errors.Join`
  (`WithCollectAll`). `errors.Is` / `errors.As` see each one. `Errors()`
  always returns the full list.
- **Cancellation:** the context from `WithContext` is cancelled on the first
  error, and `context.Cause(ctx)` returns that error. With
  `WithContinueOnError` it stays alive until `Wait` returns, so other tasks
  are not interrupted.
- No dependency on `golang.org/x/sync`.

## API

| Identifier | Purpose |
|---|---|
| `WithContext(ctx, opts...) (*Group, context.Context)` | Group with a derived context |
| `New(opts...) *Group` / `var g Group` | Group without a context (tasks get `context.Background()`) |
| `(*Group).Go(fn)` / `TryGo(fn) bool` | Start a task (blocking / non-blocking when the limit is reached) |
| `(*Group).SetLimit(n)` / `WithLimit(n)` | Maximum number of concurrent tasks (`n < 0` means no limit) |
| `(*Group).Wait() error` | Wait for all tasks; returns the first error or all of them joined |
| `(*Group).Errors() []error` | Every recorded error in completion order |
| `WithRecover(fn)` | Panic observer (`nil` = `safe.DefaultRecover`, `safe.IgnoreRecover` = silent) |
| `WithCollectAll()` | `Wait` returns `errors.Join` of all errors |
| `WithContinueOnError()` | Do not cancel the context on error |

## Usage

```go
g, ctx := errorgroup.WithContext(ctx, errorgroup.WithLimit(8))
for _, id := range ids {
	g.Go(func(ctx context.Context) error {
		return sync(ctx, id) // a panic here becomes *safe.PanicError
	})
}
if err := g.Wait(); err != nil {
	return err
}

// Run everything, report every failure.
g := errorgroup.New(errorgroup.WithCollectAll(), errorgroup.WithContinueOnError())
g.Go(checkDB)
g.Go(checkCache)
if err := g.Wait(); err != nil {
	for _, e := range g.Errors() {
		slog.Error("health check failed", "err", e)
	}
}
```

## Performance

Intel Core Ultra 5 225H, Go 1.27. One group with 8 trivial tasks plus `Wait`
takes 3.2 µs/op, 384 B/op, 11 allocs/op. The goroutines dominate the cost.

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| `SafeGroup` | `Group` (`SafeGroup` remains as a deprecated alias) |
| A panic was recorded **twice** (in `Errors()` and again by `Wait`) | Recorded once |
| `Wait` returned `fmt.Errorf("safe group errors: %v", errs)`, so `errors.Is` did not work | Returns the first error itself, or `errors.Join` with `WithCollectAll` |
| `Wait` appended to `Errors()` on every call | `Wait` does not change state; it can be called repeatedly |
| `RecoverFunc func(r any)`, `DefaultRecover(r any)` | Aliases of `safe.RecoverFunc func(*safe.PanicError)` and `safe.DefaultRecover` |
| no limit control | `SetLimit`, `TryGo`, `WithLimit` |
| depended on `golang.org/x/sync` | standard library only |
