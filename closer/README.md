# closer

[Русская версия](READMEru.md) · [← go-core](../README.md)

Graceful shutdown helper. Register resources as you open them and close them
all in reverse order with one call, on demand or on SIGINT/SIGTERM. Each
resource is closed exactly once, and the call returns even if a closer
fails, panics or hangs.

```go
import "github.com/jwm1rr0rb10/go-core/closer"
```

## Features

- **One LIFO list** for every kind of resource:
  - `io.Closer`;
  - `Close()` without an error;
  - `func(ctx) error` such as `http.Server.Shutdown`.

  They are closed in strict reverse registration order.
- **Bounded shutdown:**
  - `WithTimeout` limits the whole `Close`;
  - `WithCloserTimeout` limits each closer.

  A closer that overruns is reported and left running in the background, and
  the next one starts.
- **Panics in closers are recovered** and reported as `*safe.PanicError`.
  The remaining resources are still closed.
- **Idempotent** `Close`: concurrent and repeated calls return the first
  result.
- **Named errors:** `close db: connection reset`, all joined with
  `errors.Join`.
- **Late registration:** a resource added after `Close` started is closed
  immediately, so it does not leak.
- **Signals:** `CloseOnSignal` waits for SIGINT/SIGTERM. After the first
  signal the default handler is restored, so a **second Ctrl+C kills a hung
  shutdown**.
- Silent by default; optional `*slog.Logger`.

## API

| Identifier | Purpose |
|---|---|
| `NewLIFOCloser(opts...)` / `var lc LIFOCloser` | Create |
| `Add(...io.Closer)`, `AddNamed(name, io.Closer)` | Register closers that return an error |
| `AddNoErr(...NoErrCloser)` | Register `Close()` without an error |
| `AddFunc(name, func(ctx) error)` | Register a context-aware close function |
| `Close() error`, `CloseContext(ctx) error` | Close everything (LIFO), joined errors |
| `Len() int` | Resources still registered |
| `WithTimeout(d)`, `WithCloserTimeout(d)`, `WithLogger(l)` | Options |
| `CloseOnSignal(lc, sigs...)` | Block until a signal, then close |
| `CloseOnSignalWithContext(ctx, lc, sigs...)` | Same, or when `ctx` is done |
| `CloseOnSignalContext(lc, sigs...)` | Returns `func(ctx) error` for run-groups |
| `DefaultSignals` | `os.Interrupt`, `syscall.SIGTERM` |
| `CloserFunc`, `NoErrCloserFunc` | Function adapters |

## Usage

```go
func main() {
	lc := closer.NewLIFOCloser(
		closer.WithTimeout(20*time.Second),
		closer.WithCloserTimeout(5*time.Second),
		closer.WithLogger(slog.Default()),
	)

	db := mustOpenDB()
	lc.AddNamed("postgres", db)

	nc := mustConnectNATS()
	lc.AddNoErr(nc) // (*nats.Conn).Close()

	srv := &http.Server{Addr: ":8080", Handler: router}
	lc.AddFunc("http", srv.Shutdown) // stopped first: last in, first out
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http", "err", err)
		}
	}()

	if err := closer.CloseOnSignal(lc); err != nil {
		slog.Error("shutdown finished with errors", "err", err)
		os.Exit(1)
	}
}
```

## Notes

- `CloseOnSignalWithContext`'s `ctx` only **triggers** shutdown. How long
  closing may take is controlled by `WithTimeout`, so a cancelled parent
  context does not cut shutdown short.
- A closer must not call `Close` on the same `LIFOCloser`: that call would
  wait for itself. It may call `Add*`.
- Without any timeout, closers run inline on the calling goroutine, with no
  extra goroutines.

Benchmark (Intel Core Ultra 5 225H, Go 1.27): registering and closing 8
resources takes 0.96 µs, 1032 B, 21 allocs. This is not a hot path.

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| Error closers and no-error closers lived in two separate lists, so the real order was not LIFO | One list, strict LIFO across all kinds |
| `Close` could run twice and close resources twice | Idempotent; later calls return the first error |
| No timeouts: one hung closer blocked shutdown forever | `WithTimeout`, `WithCloserTimeout`, `CloseContext(ctx)` |
| A panic in a closer crashed shutdown | Recovered and reported as `*safe.PanicError` |
| Errors were `close error: <err>` with no resource name | `close <name>: <err>` |
| `CloseOnSignal` called `log.Printf` and had `defer close(ch)` on a channel registered with `signal.Notify` | No global logging (`WithLogger`), built on `signal.NotifyContext`, a second signal terminates the process |
| No signals given meant waiting forever | No signals given means `DefaultSignals` (SIGINT, SIGTERM) |
| `NewLIFOCloser()` | `NewLIFOCloser(opts...)`; the zero value also works |
| none | `AddNamed`, `AddFunc`, `CloseContext`, `Len` |
