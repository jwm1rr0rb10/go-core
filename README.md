# go-core

[Русская версия](READMEru.md)

Shared building blocks for Go backend services: authentication, retries, graceful
shutdown, panic-safe concurrency, high-load TCP, UUIDs, validation and small generic
helpers. Every package is race-tested, benchmarked and documented in English and Russian.

```bash
go get github.com/jwm1rr0rb10/go-core@latest
```

Requires Go 1.27+. Import only the packages you need — each one is independent, and
heavy dependencies (gRPC, websocket, go-playground/validator) are pulled in only by the
packages that use them.

---

## Packages

| Package | What it does | Docs |
|---|---|---|
| [`api/jwt`](api/jwt) | HS256 access/refresh token pairs with enforced token type, one-time refresh tokens, `net/http` middleware and gRPC unary/stream interceptors | [EN](api/jwt/README.md) · [RU](api/jwt/READMEru.md) |
| [`array`](array) | Generic slice helpers (map/filter/group/zip/chunk/sort…) and lazy `iter.Seq` variants; never mutate input | [EN](array/README.md) · [RU](array/READMEru.md) |
| [`bytes`](bytes) | JSON ↔ Go values: generic `Marshal`/`Unmarshal`, `map[string]any` helpers with exact big integers | [EN](bytes/README.md) · [RU](bytes/READMEru.md) |
| [`clock`](clock) | `Clock` interface mirroring `time` (timers, tickers, `AfterFunc`) and a deterministic `Mock` for tests | [EN](clock/README.md) · [RU](clock/READMEru.md) |
| [`closer`](closer) | Graceful shutdown: LIFO resource closing with timeouts, panic recovery and signal handling | [EN](closer/README.md) · [RU](closer/READMEru.md) |
| [`healthcheck`](healthcheck) | `grpc.health.v1` server driven by dependency status and polled probes | [EN](healthcheck/README.md) · [RU](healthcheck/READMEru.md) |
| [`pointer`](pointer) | Generic helpers for optional values as pointers (`ToPointer`, `ValueOr`, `Coalesce`, `Equal`…) | [EN](pointer/README.md) · [RU](pointer/READMEru.md) |
| [`random`](random) | Fast (`math/rand/v2`) and secure (`crypto/rand`) random numbers, strings, tokens, times, IPs | [EN](random/README.md) · [RU](random/READMEru.md) |
| [`repeat`](repeat) | Retries with capped exponential backoff and jitter, retrying `http.RoundTripper`, WebSocket dialer | [EN](repeat/README.md) · [RU](repeat/READMEru.md) |
| [`safe`](safe) | Turns panics into typed `*PanicError` values; panic-safe goroutines and calls | [EN](safe/README.md) · [RU](safe/READMEru.md) |
| [`safe/errorgroup`](safe/errorgroup) | Panic-safe `errgroup` with limits and first-error / collect-all modes | [EN](safe/errorgroup/README.md) · [RU](safe/errorgroup/READMEru.md) |
| [`safe/waitgroup`](safe/waitgroup) | Lock-free `WaitGroup` that reports misuse and waits with context or timeout | [EN](safe/waitgroup/README.md) · [RU](safe/waitgroup/READMEru.md) |
| [`tcp`](tcp) | High-load TCP server, context-aware client with framing and retries, pool with liveness checks, sharded rate limiter, proof-of-work | [EN](tcp/README.md) · [RU](tcp/READMEru.md) |
| [`time`](time) | Unix timestamps of unknown unit, cached time zones, minute offsets, duration formatting, `slog` timing | [EN](time/README.md) · [RU](time/READMEru.md) |
| [`uuid`](uuid) | RFC 9562 UUID type: fast parse/format, JSON/SQL support, lock-free v4 and monotonic v7 | [EN](uuid/README.md) · [RU](uuid/READMEru.md) |
| [`uuid/db`](uuid/db) | Crypto-random v4 / monotonic v7 constructors for database keys | [EN](uuid/db/README.md) · [RU](uuid/db/READMEru.md) |
| [`uuid/network`](uuid/network) | Very fast non-secret UUIDs for request, trace and message IDs | [EN](uuid/network/README.md) · [RU](uuid/network/READMEru.md) |
| [`uuid/google_uuid`](uuid/google_uuid) | String ID generators behind one interface: google/uuid v4/v7 and monotonic ULID | [EN](uuid/google_uuid/README.md) · [RU](uuid/google_uuid/READMEru.md) |
| [`validator`](validator) | Struct-tag `Engine` over go-playground/validator plus composable value checks, one error type | [EN](validator/README.md) · [RU](validator/READMEru.md) |

> `time` and `bytes` share names with standard packages. Import them with an alias:
> `coretime "github.com/jwm1rr0rb10/go-core/time"`.

---

## Quick start

### Authentication (HTTP + gRPC)

```go
auth, err := jwt.NewHelper([]byte(os.Getenv("JWT_SECRET")), // at least 32 bytes
	jwt.WithIssuer("billing"),
	jwt.WithAccessTTL(5*time.Minute),
	jwt.WithRefreshStore(jwt.NewMemoryRefreshStore()), // one-time refresh tokens
)
if err != nil {
	log.Fatal(err)
}

mux.Handle("/api/", auth.HTTPMiddleware()(handler))
mux.Handle("/admin/", auth.HTTPMiddleware(jwt.WithRoles(1, 2))(adminAPI))

interceptor := jwt.NewAuthInterceptor(auth,
	jwt.WithMethodRoles(map[string][]uint64{"/billing.v1.Billing/Refund": {1}}),
	jwt.WithPublicMethods("/billing.v1.Billing/Ping"),
)
srv := grpc.NewServer(
	grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor()),
	grpc.StreamInterceptor(interceptor.StreamServerInterceptor()),
)
```

### Retries

```go
err := repeat.Exec(ctx, func(ctx context.Context, attempt int) error {
	return doWork(ctx)
}, repeat.WithMaxAttempts(5), repeat.WithBackoff(100*time.Millisecond, 5*time.Second))

body, err := repeat.Do(ctx, fetch, repeat.WithMaxElapsed(30*time.Second))

client := repeat.NewClient(nil) // retries idempotent requests, honors Retry-After
```

### Concurrency without crashes

```go
g, ctx := errorgroup.WithContext(ctx, errorgroup.WithLimit(16))
for _, u := range urls {
	g.Go(func(ctx context.Context) error { return download(ctx, u) })
}
err := g.Wait() // a panic in a task comes back as *safe.PanicError
```

### TCP server and graceful shutdown

```go
srv, err := tcp.NewServer(":9000",
	func(ctx context.Context, conn net.Conn) { _, _ = io.Copy(conn, conn) },
	tcp.WithMaxConnections(50_000),
	tcp.WithIdleTimeout(2*time.Minute),
	tcp.WithMiddleware(tcp.RateLimitMiddleware(tcp.NewRateLimiter())),
)
if err != nil {
	log.Fatal(err)
}
go srv.ListenAndServe()

lc := closer.NewLIFOCloser(closer.WithTimeout(30 * time.Second))
lc.AddFunc("tcp server", srv.Shutdown)
lc.AddFunc("http server", httpSrv.Shutdown)
if err := closer.CloseOnSignal(lc, syscall.SIGINT, syscall.SIGTERM); err != nil {
	log.Printf("shutdown: %v", err)
}
```

### IDs

```go
id := uuid.NewV7()               // time-ordered, strictly monotonic, 0 allocs
fast := network.NewV4()          // request/trace IDs, not for secrets
token, err := random.SecureToken(32) // 32 bytes from crypto/rand as 64 hex chars
```

---

## Design principles

- **Safe by default.** Finite retries, enforced token types, strong-secret checks,
  crypto randomness where it matters, no panics on misuse.
- **No hidden global state.** Everything configurable lives in an instance built with
  functional options; package-level helpers use goroutine-safe defaults.
- **Quiet libraries.** No logging on hot paths; components that can log accept an optional
  `*slog.Logger` and discard output by default.
- **Measured performance.** Hot paths have benchmarks (`make bench`); most are zero-allocation.
- **Standard interop.** Errors work with `errors.Is` / `errors.As`, `context` is honored
  everywhere, types implement `encoding` and `database/sql` interfaces.

## Development

```bash
make check   # gofmt + go vet + go test -race
make cover   # race tests with coverage
make bench   # benchmarks with -benchmem
make fuzz    # fuzz targets (array, jwt, uuid)
make lint    # golangci-lint (config in .golangci.yml)
```

CI (`.github/workflows/ci.yml`) runs gofmt, vet, race tests with coverage and golangci-lint.
Test coverage is 92–100% per package.

## Changelog

### Unreleased — major rework

Breaking changes in almost every package. Each package README has a **Migration** section
listing what changed and how to update.

- Module path is now `github.com/jwm1rr0rb10/go-core`; private `kalipso/...` imports removed.
- `api/jwt`: token type claim enforced (refresh tokens no longer work as access tokens),
  standard registered claims, one-time refresh tokens, secret length check, gRPC interceptors,
  correct gRPC status codes. **Tokens issued by older versions are rejected.**
- Data races fixed in `uuid/network`, `uuid/google_uuid`, `random`, `healthcheck`, `tcp`.
- `repeat`: finite retries by default, correct backoff/jitter, request bodies preserved on retry,
  `Retry-After` support.
- `tcp`: graceful `Shutdown(ctx)`, accept backoff, real pool liveness checks, sharded token-bucket
  rate limiter, replay-resistant proof-of-work, zero-allocation framing.
- `closer`: true LIFO order, idempotent close, per-closer timeouts and panic recovery.
- `clock`: API mirrors `time`; `Mock` is a real fake clock.
- `uuid`: one shared `UUID` type, RFC 9562 monotonic v7, parsing and SQL/JSON support.
- Removed dependencies on `grpc-ecosystem/go-grpc-middleware` and `golang.org/x/sync`.
- Added Makefile, CI workflow and golangci-lint config.

### 1.3.1 and earlier

Released under the previous module path; see git history.
