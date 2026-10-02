# safe/waitgroup

[Русская версия](READMEru.md) · [← safe](../README.md) · [← go-core](../../README.md)

A `sync.WaitGroup` that reports misuse instead of breaking, exposes its
counter, and can wait with a context or a timeout.

```go
import "github.com/jwm1rr0rb10/go-core/safe/waitgroup"
```

## Features

- **Misuse is rejected, not applied.** An `Add`/`Done` that would make the
  counter negative (or overflow `int32`) returns an error and leaves the
  counter unchanged, so the group keeps working. With `WithPanicOnMisuse`
  such calls panic instead.
- `Go(fn)`, like `sync.WaitGroup.Go`.
- `WaitContext(ctx)` and `WaitTimeout(d)`.
- `Count()` reads the counter.
- Lock-free: atomics only, ~34 ns per `Add`+`Done` pair, zero allocations.
  The zero value is ready to use.

## API

| Identifier | Purpose |
|---|---|
| `NewWaitGroup(opts...)` / `var wg WaitGroup` | Create |
| `Add(delta) error`, `Done() error` | Checked counter updates |
| `Go(fn)` | Run `fn` in a tracked goroutine (panics are not recovered; wrap with `safe.Func` if needed) |
| `Wait()` | Block until the counter is zero |
| `WaitContext(ctx) error` | Wait or return `ctx.Err()` |
| `WaitTimeout(d) bool` | `true` if the group finished in time |
| `Count() int` | Current counter |
| `WithPanicOnMisuse()` | Panic instead of returning an error |

## Usage

```go
var wg waitgroup.WaitGroup
for _, job := range jobs {
	wg.Go(func() { process(job) })
}
if !wg.WaitTimeout(30 * time.Second) {
	slog.Warn("workers still running", "count", wg.Count())
}
```

## Notes

The `sync.WaitGroup` rule still applies: an `Add` with a positive delta that
starts while the counter is zero must happen before `Wait`. If `WaitContext`
returns because of the context, a helper goroutine keeps waiting until the
group finishes.

Internally the inner `sync.WaitGroup` is incremented before the public
counter and decremented after it. The inner counter is therefore never below
the public one, and `Wait` cannot be released early.

## Performance

Intel Core Ultra 5 225H, Go 1.27:

| Benchmark | ns/op | allocs/op |
|---|---:|---:|
| `Add(1)` + `Done()` | 34 | 0 |
| same, `RunParallel` on 14 threads | 202 | 0 |

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| A rejected `Done()` still decremented the counter, which stayed negative and broke the group | The counter is left unchanged |
| A `sync.RWMutex` guarded every `Add`/`Done` | Lock-free CAS |
| `Count() int32` | `Count() int` |
| none | `Go`, `WaitContext`, `WaitTimeout`, usable zero value |
