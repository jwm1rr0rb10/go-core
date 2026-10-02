# healthcheck

[← go-core](../README.md) · [Русская версия](READMEru.md)

A `grpc.health.v1` server whose status is derived from the health of your
dependencies. Report dependency health push-style (`SetStatus`) or register
probes that are polled on an interval (`AddChecker`). The overall status is
`SERVING` only while every dependency is healthy, and individual gRPC services
can be bound to a subset of dependencies.

```go
import "github.com/jwm1rr0rb10/go-core/healthcheck"
```

## Features

- Drop-in `grpc_health_v1.HealthServer` (embeds `*health.Server`: `Check`, `List`, `Watch`)
- Push (`SetStatus`) and pull (`AddChecker`) dependency reporting
- Status changes apply immediately, not on the next tick
- Per-service status with `BindService`
- Checkers run concurrently, each with a timeout; panics are recovered and
  reported as failures
- Idempotent `Start`, `Stop`; `Shutdown` flips everything to `NOT_SERVING` for
  graceful draining; `Resume` restores it
- Watchers are notified only on real transitions
- Optional `slog` logging of transitions (silent by default)
- Race-free: all state behind one mutex, no goroutines besides the poll loop

## API overview

| Identifier | Purpose |
|---|---|
| `NewGRPCHealthServer(...Option)` | Create a server (`SERVING` with no dependencies) |
| `WithInterval`, `WithCheckTimeout`, `WithLogger` | Options (defaults 10 s, 2 s, discard) |
| `SetStatus(dep, healthy)` | Push a dependency status |
| `AddChecker(dep, CheckFunc)` | Register a polled probe (unhealthy until first success) |
| `RemoveDependency(dep)` | Forget a dependency and its probe |
| `BindService(service, deps...)` | Status of a named service = all listed deps healthy |
| `Start(ctx)` / `Stop()` | Run / stop the polling loop |
| `CheckNow(ctx)` | Run all probes once, synchronously |
| `Shutdown()` / `Resume()` | Force `NOT_SERVING` for draining / restore |
| `Statuses()`, `Errors()` | Snapshots for logs or debug endpoints |
| `HealthCheck(ctx, interval)` | Deprecated: `WithInterval` + `Start` |

## Usage

```go
hs := healthcheck.NewGRPCHealthServer(
	healthcheck.WithInterval(5*time.Second),
	healthcheck.WithCheckTimeout(time.Second),
	healthcheck.WithLogger(slog.Default()),
)

hs.AddChecker("postgres", db.PingContext)
hs.AddChecker("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
hs.BindService("orders.Orders", "postgres")

nc.SetDisconnectErrHandler(func(*nats.Conn, error) { hs.SetStatus("nats", false) })
nc.SetReconnectHandler(func(*nats.Conn) { hs.SetStatus("nats", true) })

srv := grpc.NewServer()
grpc_health_v1.RegisterHealthServer(srv, hs)

hs.Start(ctx)

// graceful shutdown
hs.Shutdown()                 // balancers stop routing new traffic
time.Sleep(drainDelay)
srv.GracefulStop()
```

Probe from the command line: `grpc_health_probe -addr=:50051 -service=orders.Orders`.

## Performance and concurrency

Measured on Intel Core Ultra 5 225H, Go 1.27.1:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| SetStatus (20 deps, no transition) | 169 | 0 | 0 |
| Check | 52 | 48 | 1 |

All methods are safe for concurrent use. `SetStatus` costs O(deps + bound
services) and calls into `health.Server` only when a status actually changes.
`Check` / `Watch` are served by the embedded `health.Server` without touching
this package's lock.

## Migration (from 1.3.x)

- `NewGRPCHealthServer()` now accepts options; existing calls still compile.
- `SetStatus` updates the served status **immediately**; before, the change was
  visible only after the next tick.
- `HealthCheck(ctx, interval)` is deprecated but works. Calling it twice no
  longer starts a second goroutine.
- With no dependencies the server reports `SERVING` right away (before it was
  `SERVING` from `health.NewServer` and stayed so until the first tick).
- Fixed: a data race (status flag written under a read lock) and duplicate
  polling goroutines.
