# tcp

[← go-core](../README.md) · [Русская версия](READMEru.md)

Building blocks for high-load TCP services: a server with connection limits,
idle timeouts, middleware and graceful shutdown; a context-aware client with
TLS, framing and retries; a client pool with real liveness checks; and a
sharded per-IP rate limiter with adaptive proof-of-work. Only the standard
library and [`go-errors`](https://github.com/jwm1rr0rb10/go-errors) are used,
and the hot paths do not allocate.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/tcp"
```

## Features

- **Server.** It has an exact connection limit that can be changed at
  runtime, and backs off on accept errors (5 ms → 1 s, so EMFILE does not
  burn CPU). Other features:
  - a sliding idle timeout;
  - panic recovery in handlers;
  - a middleware chain;
  - TLS;
  - `Serve(listener)` / `ListenAndServe` / `Start`;
  - `Shutdown(ctx)`, which closes listeners, cancels the handler context,
    waits, then force-closes stragglers;
  - lock-free per-connection statistics.
- **Client.** Every operation takes a `context.Context` (deadline *and*
  cancellation). It also offers:
  - TLS through `tls.Dialer`;
  - buffered reads with zero-allocation `ReadInto`;
  - length-prefixed frames;
  - `Do` for retrying a whole request/response exchange with exponential
    backoff and full jitter.

  `Close` is final: a closed client is never revived.
- **Pool.** LIFO reuse with a max-active limit (`Get(ctx)` waits for a slot)
  and a max-idle limit. Idle timeout and max lifetime are supported. A
  **non-blocking `MSG_PEEK` liveness check** detects connections the peer
  closed, reset or left unread data on. `Put` is safe after `Close`, and
  `Discard` handles broken clients.
- **RateLimiter.** Two token buckets per peer: above the soft rate the peer
  must solve proof of work, above the hard rate it is banned. Difficulty
  adapts (+1 bit per challenge and decays over time). Peers are grouped by
  IPv6 /64 and memory is bounded. State is split over 64 shards with
  pointer-free maps (not scanned by the GC), and the clock can be injected
  for tests.
- **Proof of work.** Challenges are SHA-256 with leading-zero-bit difficulty.
  Each one has a 128-bit random prefix and an expiry, so solutions cannot be
  replayed. Verification is allocation-free. `SolvePoW` uses all cores and
  `AnswerPoW` implements the client side.
- **Framing.** `WriteFrame` / `ReadFrame` use a 4-byte big-endian length
  prefix. Writes are single and coalesced, buffers are reused, and the length
  is validated before allocating.

## API overview

| Area | API |
|---|---|
| Server | `NewServer(addr, HandlerFunc, ...ServerOption)`, `Start`, `ListenAndServe`, `Serve(net.Listener)`, `Shutdown(ctx)`, `Close`, `Stats`, `SetMaxConnections`, `Addr` |
| Server options | `WithMaxConnections`, `WithIdleTimeout`, `WithMiddleware`, `WithServerTLS`, `WithServerLogger`, `WithListenConfig`, `WithBaseContext` |
| Client | `NewClient`, `Dial`, `Connect`, `Read`, `ReadInto`, `ReadFull`, `Write`, `ReadFrame`, `WriteFrame`, `Do`, `WriteWithRetry`, `ReadWithRetry`, `Reconnect`, `Close`, `Stats` |
| Client options | `WithTimeouts`, `WithDialTimeout`, `WithDialer`, `WithBufferSize`, `WithMaxFrameSize`, `WithTLSClientConfig`, `WithRetryPolicy`, `WithClientLogger` |
| Pool | `NewPool(Factory, ...PoolOption)`, `Get(ctx)`, `Put`, `Discard`, `Prune`, `Close`, `Stats` |
| Pool options | `WithMaxActive`, `WithMaxIdle`, `WithPoolIdleTimeout`, `WithPoolMaxLifetime`, `WithPoolHealthCheck`, `WithPoolLogger` |
| Rate limiter | `NewRateLimiter(...RateLimiterOption)`, `Check(netip.Addr)`, `Middleware()`, `Cleanup`, `Len`, `Stop` |
| Limiter options | `WithConnectionRate`, `WithBanRate`, `WithBanDuration`, `WithPoW`, `WithPoWHandshake`, `WithPoWDifficulty`, `WithPoWTimeout`, `WithDifficultyDecay`, `WithIPv6PrefixLen`, `WithMaxTrackedPeers`, `WithCleanupInterval`, `WithClock`, `WithRateLimiterLogger` |
| Proof of work | `NewPoWChallenge`, `(*PoWChallenge).Verify`, `SolvePoW`, `AnswerPoW`, `WritePoWChallenge`, `ParsePoWChallenge`, `WritePoWSolution`, `ReadPoWSolution` |
| Framing | `WriteFrame`, `ReadFrame`, `FrameHeaderSize`, `DefaultMaxFrameSize` |
| Errors | `IsRetryable`, `ConnectionError`, `ErrConnectionClosed`, `ErrTimeout`, `ErrClientClosed`, `ErrServerClosed`, `ErrPoolClosed`, `ErrFrameTooLarge`, `ErrBanned`, `ErrRateLimited`, `ErrPoWFailed` |
| TLS | `ServerTLSConfig(cert, key)`, `ClientTLSConfig(insecure)` |

## Usage

### Server with graceful shutdown

```go
srv, err := tcp.NewServer(":9000", func(ctx context.Context, conn net.Conn) {
	var buf []byte
	for {
		var err error
		if buf, err = tcp.ReadFrame(conn, buf[:0], 1<<20); err != nil {
			return // EOF, idle timeout, or shutdown (ctx is cancelled and conns are closed)
		}
		if err := tcp.WriteFrame(conn, buf, 1<<20); err != nil {
			return
		}
	}
},
	tcp.WithMaxConnections(50_000),
	tcp.WithIdleTimeout(2*time.Minute),
	tcp.WithServerLogger(slog.Default()),
)
if err != nil {
	return err
}
go func() {
	if err := srv.ListenAndServe(); !errors.Is(err, tcp.ErrServerClosed) {
		slog.Error("serve", "err", err)
	}
}()

<-ctx.Done() // e.g. signal.NotifyContext
shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx) // waits for handlers, force-closes after 15s
```

### Client with retries

```go
client, err := tcp.Dial(ctx, "10.0.0.5:9000",
	tcp.WithTimeouts(2*time.Second, 2*time.Second),
	tcp.WithRetryPolicy(tcp.RetryPolicy{MaxAttempts: 4, BaseDelay: 50 * time.Millisecond, MaxDelay: time.Second}),
)
if err != nil {
	return err
}
defer client.Close()

var resp []byte
err = client.Do(ctx, func(ctx context.Context, c *tcp.Client) error {
	if err := c.WriteFrame(ctx, req); err != nil {
		return err
	}
	var err error
	resp, err = c.ReadFrame(ctx, resp[:0])
	return err
})
```

### Pool

```go
pool, _ := tcp.NewPool(func(ctx context.Context) (*tcp.Client, error) {
	return tcp.Dial(ctx, addr)
}, tcp.WithMaxActive(256), tcp.WithMaxIdle(64),
	tcp.WithPoolIdleTimeout(time.Minute), tcp.WithPoolMaxLifetime(30*time.Minute))
defer pool.Close()

c, err := pool.Get(ctx) // waits for a slot when 256 clients are in use
if err != nil {
	return err
}
if err := c.WriteFrame(ctx, req); err != nil {
	pool.Discard(c) // broken: close it and free the slot
	return err
}
pool.Put(c)
```

### Rate limiting with proof of work

```go
rl := tcp.NewRateLimiter(
	tcp.WithConnectionRate(20, 40), // 20/s per IP, burst 40, then PoW
	tcp.WithBanRate(100, 200),      // beyond that: ban for a minute
	tcp.WithPoWDifficulty(16, 24),
	tcp.WithPoWHandshake(true),     // clients use tcp.AnswerPoW
)
defer rl.Stop()

srv, _ := tcp.NewServer(":9000", handler, tcp.WithMiddleware(rl.Middleware()))
go srv.ListenAndServe()

// client side
conn, _ := net.Dial("tcp", "server:9000")
if err := tcp.AnswerPoW(ctx, conn); err != nil { // "OK", or solve + "OK"
	return err
}
```

Wire protocol (text lines, at most 256 bytes):

```
server → POW prefix=<32 hex> difficulty=<bits> expires=<unix seconds>
client → SOLUTION nonce=<1..64 chars>          # SHA-256(prefix+nonce) has ≥ bits leading zero bits
server → OK                                     # only with WithPoWHandshake
```

Without `WithPoWHandshake`, unthrottled connections see no extra bytes, so
existing protocols keep working. Bytes the client sends right after its
solution are preserved for the handler.

## Concurrency and performance

- Every exported type is safe for concurrent use. A `Client` supports one
  reader and one writer at a time, like `net.Conn`.
- No logging on hot paths. Loggers default to `slog.DiscardHandler`.
- Server byte and activity counters are per connection: busy connections
  never contend on a shared cache line, and `Stats()` sums them on demand.
- The sliding idle deadline is refreshed at most once per `idle/16`. A
  handler that sets its own deadline suspends auto-refresh for that
  direction until it clears the deadline with a zero time.
- The rate limiter uses 64 independently locked, cache-line padded shards.
  Its maps contain no pointers.

Benchmarks (Go 1.27.1, linux/amd64, Intel Core Ultra 5 225H, 14 threads; `go test -bench . -benchmem`):

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Client ↔ Server framed echo over loopback (128 B) | 7 600 – 10 300 | 0 | 0 |
| `WriteFrame` + `ReadFrame` (512 B, in memory) | 43 – 48 | 0 | 0 |
| Server connection wrapper, per `Read` | 92 – 116 | 0 | 0 |
| `RateLimiter.Check`, one hot peer | 110 – 120 | 0 | 0 |
| `RateLimiter.Check`, parallel, many peers | 37 – 40 | 0 | 0 |
| `PoWChallenge.Verify` | 99 – 106 | 0 | 0 |
| `SolvePoW` at 16 bits (all cores) | ~1.3 ms | — | — |
| `IsRetryable` | 4.8 | 0 | 0 |

Each extra PoW bit doubles the client's expected work: 16 bits ≈ 65 k
hashes, 24 bits ≈ 16.7 M hashes (a few seconds on one core). Verification
always costs one hash.

## Security notes

- **Proxies and load balancers.** The limiter keys on `conn.RemoteAddr()`.
  Behind an L4 balancer that is the balancer's IP, so all clients would share
  one bucket. Enable the PROXY protocol on the balancer and wrap the listener
  (for example with `github.com/pires/go-proxyproto`) before `Serve`.
- **Non-IP connections.** The limiter rejects connections whose remote
  address is not an IP (for example Unix sockets).
- **Saturation.** When the peer table is full (`WithMaxTrackedPeers`), new
  peers must solve PoW at the maximum difficulty. They are rejected instead
  if PoW is disabled.
- **Replay.** Challenges are per connection, bound to 128 random bits and
  expire after `WithPoWTimeout`, so solutions cannot be replayed. The PoW
  exchange runs under its own deadline, which defeats slow-loris clients.
- **Frame limits.** `ReadFrame` validates the length prefix before
  allocating, so a hostile 4 GiB length costs nothing.
- **TLS.** `ClientTLSConfig(true)` disables certificate verification. Use it
  only in tests. Both helpers enforce TLS 1.2+.
- **Retries.** They are at least once. Use `Do` and `WriteWithRetry` only for
  idempotent requests.

## Migration from 1.x

| Before | Now |
|---|---|
| `NewServer(addr, func(net.Conn), tlsCfg, opts...)` | `NewServer(addr, func(ctx, net.Conn), opts...)` + `WithServerTLS(cfg)`; the handler gets a context that is cancelled on shutdown |
| `WithServerLogger(*log.Logger)` (also client/pool) | `*slog.Logger`; default is discard, not `log.Default()` |
| `WithMiddleware(func(net.Conn) bool)`, `ApplyMiddleware` | `WithMiddleware(...Middleware)` where `Middleware func(ctx, net.Conn) (net.Conn, error)`; the server closes rejected connections; `ApplyMiddleware` removed |
| `Stop()`, `StopWithTimeout(d)` | `Shutdown(ctx)` / `Close()`; the old methods are deprecated wrappers that no longer leak goroutines. A server cannot be restarted |
| `WithServerTimeout(d)` (absolute deadline) | `WithIdleTimeout(d)`, a sliding idle timeout (old name kept as a deprecated alias) |
| maximum 65101 connections by default | unlimited by default; set `WithMaxConnections` |
| `NewClient(addr, tlsCfg, opts...)` | `NewClient(addr, opts...)` + `WithTLSClientConfig(cfg)`; also `Dial(ctx, addr, opts...)` |
| `Connect()`, `Read()`, `Write(b)`, `Reconnect()` | take `ctx` first. `Connect` is a no-op when already connected; `Close` is final and `Reconnect` after it returns `ErrClientClosed` |
| `WriteWithRetry(data, n, backoff)`, `ReadWithRetry(n, backoff)` | `WriteWithRetry(ctx, data)`, `ReadWithRetry(ctx)` + `WithRetryPolicy`; prefer `Do` |
| `ConnectionStats.RetryCount` (reset on success) | total retries; new `Reconnects` field |
| default read buffer 1024 | 4096 (`WithBufferSize`) |
| `NewConnectionPool(func() (*Client, error), size)`, `ConnectionPool`, `Get()` | `NewPool(func(ctx) (*Client, error), opts...)`, `Pool`, `Get(ctx)`; new `Discard`, `Prune`, `Stats`; `Close` returns an error; `WithPoolPingTimeout` removed (the check never blocks) |
| `NewRateLimiter(*log.Logger)`, fixed 1-second window | `NewRateLimiter(opts...)` with token buckets; `RateLimitMiddleware(rl)` is deprecated, use `rl.Middleware()` |
| PoW difficulty in **hex digits** (default 4..8) | difficulty in **bits** (default 16..24; old 4 digits = 16 bits) |
| `GeneratePoWChallenge(d)`, `PoWSolution`, `ValidatePoWSolution` | `NewPoWChallenge(bits, ttl)`, `(*PoWChallenge).Verify(nonce)`; `ReadPoWSolution(*bufio.Reader) (string, error)`; challenge line now carries `expires=` |
| exported constants `TCP`, `Read`, `Write` | removed |
| error messages `"connection closed"` | prefixed with `"tcp: "`; compare with `errors.Is`, never by string |
