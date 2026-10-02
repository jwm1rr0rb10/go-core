# clock

[Русская версия](READMEru.md) · [← go-core](../README.md)

A time abstraction for code that uses timers, tickers and timeouts. Production
code depends on the `Clock` interface and gets the real implementation from
`clock.New()`; tests pass a `clock.Mock`, whose time moves only when the test
says so. No sleeps in tests, no flaky timeouts.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/clock"
```

## Features

- Mirrors the `time` package: `Now`, `Since`, `Until`, `Sleep`, `After`,
  `NewTimer`, `AfterFunc`, `NewTicker`, plus `SleepContext`.
- Same edge-case semantics as `time`: a zero or negative duration fires
  immediately, `NewTicker(0)` panics, and `Stop`/`Reset` never leave a stale
  value in the channel (Go 1.23+ timer behaviour).
- `Mock` is a real fake clock: `Advance` fires every due timer and ticker in
  deadline order, and `Now()` inside a callback reports that timer's deadline.
- `BlockUntil(n)` waits until the code under test has registered `n` timers or
  sleepers, so a test can advance the clock at exactly the right moment.
- Everything is safe for concurrent use; the mock runs no background goroutines.

## API

| Item | Description |
|------|-------------|
| `type Clock` | `Now`, `Since`, `Until`, `Sleep`, `SleepContext`, `After`, `NewTimer`, `AfterFunc`, `NewTicker` |
| `type Timer` | `C() <-chan time.Time`, `Stop() bool`, `Reset(d) bool` |
| `type Ticker` | `C() <-chan time.Time`, `Stop()`, `Reset(d)` |
| `New() Clock` | Real clock backed by `time` |
| `NewMock(start) *Mock` | Fake clock starting at `start` |
| `(*Mock).Advance(d)` | Move forward, firing due timers (AfterFunc callbacks run synchronously) |
| `(*Mock).Set(t)` | Jump to `t`; forward fires timers, backward only changes `Now` |
| `(*Mock).BlockUntil(n)` / `BlockUntilContext(ctx, n)` | Wait for `n` active waiters |
| `(*Mock).Waiters()` | Number of active timers, tickers and sleepers |

## Usage

Production code takes a `Clock`:

```go
type Cache struct {
	clock clock.Clock
	ttl   time.Duration
}

func (c *Cache) expired(storedAt time.Time) bool {
	return c.clock.Since(storedAt) > c.ttl
}

cache := &Cache{clock: clock.New(), ttl: time.Minute}
```

A test drives time explicitly:

```go
func TestWorkerRetriesAfterDelay(t *testing.T) {
	m := clock.NewMock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	done := make(chan struct{})
	go func() {
		m.Sleep(30 * time.Second) // the code under test waits here
		close(done)
	}()

	m.BlockUntil(1)             // the goroutine is now sleeping
	m.Advance(30 * time.Second) // wakes it up instantly
	<-done
}
```

Tickers:

```go
tk := m.NewTicker(time.Minute)
defer tk.Stop()

m.Advance(time.Minute)
tick := <-tk.C() // 00:01:00
```

## Concurrency and performance

- The real clock is a zero-size value; `Now()` costs the same as `time.Now()`
  (≈33 ns, 0 allocs on the test machine).
- The mock keeps waiters in a sorted slice under one mutex. Creating a timer,
  advancing and receiving takes ≈360 ns and 5 allocations, which is plenty for
  tests.
- Like `time.Ticker`, a mock ticker drops ticks for a slow receiver instead of
  blocking `Advance`.

## Migration

The previous interface returned errors for non-positive durations and had a
`Tick` method. Changes:

| Before | Now |
|--------|-----|
| `After(d) (<-chan time.Time, error)` | `After(d) <-chan time.Time`; `d <= 0` fires immediately |
| `Sleep(d) error` | `Sleep(d)`; `d <= 0` returns immediately. Use `SleepContext` for cancellation |
| `Tick(d) (<-chan time.Time, func(), error)` | `NewTicker(d) Ticker`; call `Stop()` instead of the stop func |
| `Mock.After` / `Mock.Sleep` advanced the clock themselves | They block until the test calls `Advance` (use `BlockUntil` first) |
| `Mock.Tick` spun a busy goroutine | `Mock.NewTicker` fires only on `Advance`, no goroutines |
