# repeat

[Русская версия](READMEru.md) · [← go-core](../README.md)

Retries with capped exponential backoff and jitter, plus a retrying
`http.RoundTripper` and a WebSocket dial helper. Defaults are safe for
production: a small finite number of attempts, jittered exponential delays,
and no retry after the context is cancelled.

```go
import "github.com/jwm1rr0rb10/go-core/repeat"
```

## Features

- **Finite by default.** 4 attempts, 100 ms → 10 s capped exponential backoff
  with full jitter.
- **Four jitter strategies:** none, full, equal, decorrelated. Jitter is
  applied after the cap, so it is never clamped away.
- **Clear budgets.** `WithMaxAttempts` counts total attempts. `WithMaxElapsed`
  sets a time budget. A context deadline is respected *before* waiting: a delay
  that would cross the deadline is not slept.
- **Retry control.** `Permanent(err)` stops at once, `WithRetryIf` /
  `WithRetryPolicy` filter errors, and `RetryAfter(err, d)` lets the server
  dictate the next delay.
- **Observability hook.** `WithOnRetry(func(RetryEvent))` for logs and
  metrics. The package itself never logs.
- **Generic** `Do[T]` for operations that return a value.
- **Zero allocations** on the hot path with a prebuilt `Retrier` (immutable,
  goroutine-safe).
- **HTTP `Transport`:**
  - retries only idempotent requests by default;
  - rewinds bodies via `GetBody`;
  - honors `Retry-After`;
  - drains and closes discarded responses so connections are reused.

## API

| Identifier | Purpose |
|---|---|
| `Exec(ctx, op, opts...) error` | Run `op(ctx, attempt)` with retries |
| `Do[T](ctx, fn, opts...) (T, error)` | Same, for value-returning functions |
| `New(opts...) (*Retrier, error)` / `(*Retrier).Exec` / `DoWith` | Validate once, reuse everywhere |
| `WithMaxAttempts(n)` / `Unlimited` | Total attempts (1 = no retries) |
| `WithBackoff(base, max)`, `WithBaseDelay`, `WithMaxDelay`, `WithConstantDelay` | Delay shape |
| `WithJitter(JitterFull \| JitterNone \| JitterEqual \| JitterDecorrelated)` | Randomization |
| `WithMaxElapsed(d)` | Total time budget → `ErrMaxElapsed` |
| `WithRetryIf(fn)`, `WithRetryPolicy(fn)` | Which errors to retry |
| `WithOnRetry(fn)` | Hook before each wait (`RetryEvent{Attempt, Err, Delay}`) |
| `Permanent(err)`, `IsPermanent(err)` | Stop retrying |
| `RetryAfter(err, d)` | Minimum delay before the next attempt |
| `*Error{Attempts, Err, Cause}` | Returned when giving up; unwraps to `Err` and `Cause` |
| `Backoff(base, limit, attempt)` | Overflow-safe `min(limit, base·2ⁿ)` |
| `NewTransport(base, opts...)`, `Transport{RetryStatus, RetryNonIdempotent}` | Retrying `http.RoundTripper` |
| `NewClient(*http.Client, opts...) *http.Client` | Copy of a client with a retrying transport |
| `DefaultRetryStatus(code)` | 429, 502, 503, 504 |
| `StatusError` | Per-attempt error for a retryable status (visible in hooks) |
| `ConnectWithRetry(ctx, dial, opts...)`, `Dialer(d, url, h)` | WebSocket dialing |

## Usage

```go
// One-off call with defaults (4 attempts, jittered backoff).
err := repeat.Exec(ctx, func(ctx context.Context, attempt int) error {
	return client.Ping(ctx)
})

// Shared retrier with a policy, metrics hook and value result.
var retrier, _ = repeat.New(
	repeat.WithMaxAttempts(5),
	repeat.WithBackoff(50*time.Millisecond, 2*time.Second),
	repeat.WithMaxElapsed(10*time.Second),
	repeat.WithRetryIf(func(err error) bool { return !errors.Is(err, ErrNotFound) }),
	repeat.WithOnRetry(func(e repeat.RetryEvent) {
		retries.Inc()
		slog.Debug("retrying", "attempt", e.Attempt, "err", e.Err, "delay", e.Delay)
	}),
)

user, err := repeat.DoWith(ctx, retrier, func(ctx context.Context) (*User, error) {
	u, err := repo.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repeat.Permanent(err) // no point retrying
	}
	return u, err
})

var re *repeat.Error
if errors.As(err, &re) {
	slog.Warn("gave up", "attempts", re.Attempts, "cause", re.Cause)
}
```

HTTP:

```go
client := repeat.NewClient(&http.Client{Timeout: 10 * time.Second},
	repeat.WithMaxAttempts(3),
	repeat.WithBackoff(100*time.Millisecond, 2*time.Second),
)
resp, err := client.Get("https://api.example.com/items") // retried on 429/502/503/504 and network errors

// Or plug the transport into an existing client / SDK.
tr := repeat.NewTransport(http.DefaultTransport, repeat.WithMaxAttempts(4))
tr.RetryStatus = func(code int) bool { return code == 500 || repeat.DefaultRetryStatus(code) }
sdkClient := &http.Client{Transport: tr}
```

WebSocket:

```go
conn, err := repeat.ConnectWithRetry(ctx,
	repeat.Dialer(nil, "wss://stream.example.com/ws", nil),
	repeat.WithMaxAttempts(repeat.Unlimited),
	repeat.WithMaxElapsed(time.Minute),
)
```

## Behaviour details

- **`attempt` numbering.** `attempt` passed to the operation (and
  `RetryEvent.Attempt`) is 0-based. `Error.Attempts` is a count.
- **Return values.**
  - A rejected error (`Permanent`, `RetryIf`) is returned **as is**:
    `Permanent` is unwrapped when it is the top-level error.
  - When attempts, time or context run out, the result is `*repeat.Error`.
    `errors.Is` matches both the last error and the cause
    (`context.Canceled`, `context.DeadlineExceeded`, `ErrMaxElapsed`).
- **`Retry-After` delays.** A `RetryAfter` hint (or `Retry-After` header) can
  exceed `MaxDelay`. It is still bounded by `MaxElapsed` and the context
  deadline.
- **Transport and idempotency.**
  - Idempotent means `GET`, `HEAD`, `OPTIONS`, `TRACE`, `PUT`, `DELETE`, or
    any request with an `Idempotency-Key` / `X-Idempotency-Key` header.
  - A request whose body cannot be rewound (no `GetBody`) is sent once.
  - When retries run out, the **last response** is returned with a `nil`
    error, as `http.RoundTripper` requires.
- **Client timeout.** `http.Client.Timeout` covers all attempts together.

## Performance

Measured on an Intel Core Ultra 5 225H, Go 1.27, `-benchmem`:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `Retrier.Exec`, success | 36.8 | 0 | 0 |
| `Retrier.Exec`, 2 retries with zero delay | 68.5 | 0 | 0 |
| `Exec(ctx, op, opts...)` (builds config per call) | 74.7 | 64 | 1 |

Randomness comes from the goroutine-safe `math/rand/v2` global source. A
single `time.Timer` is reused across waits.

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| Unlimited retries by default (up to a hidden 24 h) | **4 attempts** by default. Use `WithMaxAttempts(repeat.Unlimited)` + `WithMaxElapsed` explicitly |
| `WithMaxAttempts(n)` meant *retries* (n+1 calls) | It means **total attempts** (n calls) |
| `WithMinWait` / `WithMaxWait` (random wait in a range; min overrode backoff) | `WithBackoff(base, max)` / `WithConstantDelay`. Old names are deprecated aliases for base/max delay |
| `WithExponentialBackoff`, `WithErrorFilter` | Deprecated aliases of `WithBackoff`, `WithRetryIf` |
| `WithJitter(func(time.Duration) time.Duration)` | `WithJitter(repeat.Jitter)` strategy constant |
| `Config` struct exported, `OptionSetter` | Config is internal; `Option` (with `OptionSetter` as an alias) |
| Errors: `"max attempts (n): err"` string wrap; cancellation returned bare `ctx.Err()` | `*repeat.Error` with `Attempts`, `Err` and `Cause`; `errors.Is` works for both |
| `NewClient` returned `*ClientWithRetry` | Returns `*http.Client`; `client.Do(req)` still works. Retries are done by `Transport` |
| HTTP retried every 5xx, any method, lost the request body on retry | Retries 429/502/503/504 and network errors for idempotent requests, rewinds the body, honors `Retry-After` |
| `TemporaryError` | `StatusError` (seen only by hooks; `RoundTrip` returns the last response) |
| `ConnectWithRetry` logged via `log.Printf` | Silent. Use `WithOnRetry` |
| `MaxTotalDuration`, `DefaultMinWait`, … constants | `DefaultMaxAttempts`, `DefaultBaseDelay`, `DefaultMaxDelay`, `Unlimited` |
