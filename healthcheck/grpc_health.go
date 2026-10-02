package healthcheck

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"sync"
	"time"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Defaults for the polling loop.
const (
	DefaultCheckInterval = 10 * time.Second
	DefaultCheckTimeout  = 2 * time.Second
)

// CheckFunc probes a dependency; a nil error means healthy. It must honor ctx.
type CheckFunc func(ctx context.Context) error

// Option configures a GRPCHealthServer.
type Option func(*GRPCHealthServer)

// WithInterval sets how often registered checkers are polled.
func WithInterval(d time.Duration) Option {
	return func(gs *GRPCHealthServer) {
		if d > 0 {
			gs.interval = d
		}
	}
}

// WithCheckTimeout bounds the duration of a single CheckFunc call.
func WithCheckTimeout(d time.Duration) Option {
	return func(gs *GRPCHealthServer) {
		if d > 0 {
			gs.timeout = d
		}
	}
}

// WithLogger logs dependency status transitions (never on every poll).
func WithLogger(l *slog.Logger) Option {
	return func(gs *GRPCHealthServer) {
		if l != nil {
			gs.logger = l
		}
	}
}

// GRPCHealthServer is a grpc.health.v1 server driven by dependency status.
// Register it with healthpb.RegisterHealthServer(srv, gs).
type GRPCHealthServer struct {
	*health.Server

	interval time.Duration
	timeout  time.Duration
	logger   *slog.Logger

	mu       sync.Mutex
	deps     map[string]bool
	errs     map[string]error
	checkers map[string]CheckFunc
	services map[string][]string
	last     map[string]healthpb.HealthCheckResponse_ServingStatus
	shutdown bool

	cancel context.CancelFunc
	done   chan struct{}
}

// NewGRPCHealthServer returns a server reporting SERVING until a dependency
// becomes unhealthy.
func NewGRPCHealthServer(opts ...Option) *GRPCHealthServer {
	gs := &GRPCHealthServer{
		Server:   health.NewServer(),
		interval: DefaultCheckInterval,
		timeout:  DefaultCheckTimeout,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		deps:     make(map[string]bool),
		errs:     make(map[string]error),
		checkers: make(map[string]CheckFunc),
		services: make(map[string][]string),
		last:     make(map[string]healthpb.HealthCheckResponse_ServingStatus),
	}
	for _, opt := range opts {
		opt(gs)
	}
	gs.mu.Lock()
	gs.recomputeLocked()
	gs.mu.Unlock()
	return gs
}

// SetStatus records the health of a dependency and updates the served status
// immediately.
func (gs *GRPCHealthServer) SetStatus(dependency string, healthy bool) {
	var err error
	if !healthy {
		err = errUnhealthy
	}
	gs.apply(dependency, err)
}

var errUnhealthy = fmt.Errorf("healthcheck: reported unhealthy")

// RemoveDependency forgets a dependency and its checker.
func (gs *GRPCHealthServer) RemoveDependency(dependency string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.deps, dependency)
	delete(gs.errs, dependency)
	delete(gs.checkers, dependency)
	gs.recomputeLocked()
}

// AddChecker registers a polled dependency. It counts as unhealthy until its
// first successful check (Start runs the first round immediately).
func (gs *GRPCHealthServer) AddChecker(dependency string, fn CheckFunc) {
	if fn == nil {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.checkers[dependency] = fn
	if _, ok := gs.deps[dependency]; !ok {
		gs.deps[dependency] = false
		gs.errs[dependency] = fmt.Errorf("healthcheck: %s not checked yet", dependency)
	}
	gs.recomputeLocked()
}

// BindService makes the named gRPC service SERVING only while all listed
// dependencies are healthy. Unknown dependencies count as unhealthy.
func (gs *GRPCHealthServer) BindService(service string, dependencies ...string) {
	if service == "" {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.services[service] = append([]string(nil), dependencies...)
	gs.recomputeLocked()
}

// Statuses returns a snapshot of dependency health.
func (gs *GRPCHealthServer) Statuses() map[string]bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return maps.Clone(gs.deps)
}

// Errors returns the last error of every unhealthy dependency.
func (gs *GRPCHealthServer) Errors() map[string]error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return maps.Clone(gs.errs)
}

// Start polls registered checkers every interval until ctx is cancelled or
// Stop/Shutdown is called. The first round runs immediately. Calling Start on
// a running server is a no-op.
func (gs *GRPCHealthServer) Start(ctx context.Context) {
	gs.mu.Lock()
	if gs.done != nil || gs.shutdown {
		gs.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	gs.cancel, gs.done = cancel, done
	gs.mu.Unlock()

	go func() {
		defer func() {
			gs.mu.Lock()
			if gs.done == done {
				gs.cancel, gs.done = nil, nil
			}
			gs.mu.Unlock()
			close(done)
		}()
		gs.loop(loopCtx)
	}()
}

// HealthCheck starts polling with the given interval (non-positive keeps the
// configured one).
//
// Deprecated: use WithInterval and Start.
func (gs *GRPCHealthServer) HealthCheck(ctx context.Context, checkInterval time.Duration) {
	if checkInterval > 0 {
		gs.mu.Lock()
		if gs.done == nil {
			gs.interval = checkInterval
		}
		gs.mu.Unlock()
	}
	gs.Start(ctx)
}

func (gs *GRPCHealthServer) loop(ctx context.Context) {
	gs.CheckNow(ctx)
	t := time.NewTicker(gs.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			gs.CheckNow(ctx)
		}
	}
}

// CheckNow runs every registered checker once, concurrently, and waits for
// the results.
func (gs *GRPCHealthServer) CheckNow(ctx context.Context) {
	gs.mu.Lock()
	checkers := maps.Clone(gs.checkers)
	gs.mu.Unlock()

	var wg sync.WaitGroup
	for name, fn := range checkers {
		wg.Go(func() {
			gs.apply(name, gs.runCheck(ctx, fn))
		})
	}
	wg.Wait()
}

func (gs *GRPCHealthServer) runCheck(ctx context.Context, fn CheckFunc) (err error) {
	ctx, cancel := context.WithTimeout(ctx, gs.timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("healthcheck: checker panicked: %v", r)
		}
	}()
	return fn(ctx)
}

// Stop stops the polling loop and waits for it to exit. The served status is
// left unchanged.
func (gs *GRPCHealthServer) Stop() {
	gs.mu.Lock()
	cancel, done := gs.cancel, gs.done
	gs.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// Shutdown stops polling and sets every service to NOT_SERVING; later status
// changes are ignored until Resume. Call it at the start of graceful shutdown
// so load balancers drain traffic.
func (gs *GRPCHealthServer) Shutdown() {
	gs.Stop()
	gs.mu.Lock()
	gs.shutdown = true
	gs.mu.Unlock()
	gs.Server.Shutdown()
}

// Resume undoes Shutdown and re-applies the dependency-derived status.
func (gs *GRPCHealthServer) Resume() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.shutdown = false
	gs.Server.Resume()
	clear(gs.last)
	gs.recomputeLocked()
}

func (gs *GRPCHealthServer) apply(dep string, err error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	healthy := err == nil
	if prev, ok := gs.deps[dep]; !ok || prev != healthy {
		if healthy {
			gs.logger.Info("dependency healthy", "dependency", dep)
		} else {
			gs.logger.Warn("dependency unhealthy", "dependency", dep, "error", err)
		}
	}
	gs.deps[dep] = healthy
	if healthy {
		delete(gs.errs, dep)
	} else {
		gs.errs[dep] = err
	}
	gs.recomputeLocked()
}

func (gs *GRPCHealthServer) recomputeLocked() {
	if gs.shutdown {
		return
	}
	all := true
	for _, ok := range gs.deps {
		if !ok {
			all = false
			break
		}
	}
	gs.setLocked("", all)
	for svc, deps := range gs.services {
		ok := true
		for _, d := range deps {
			if !gs.deps[d] {
				ok = false
				break
			}
		}
		gs.setLocked(svc, ok)
	}
}

func (gs *GRPCHealthServer) setLocked(service string, ok bool) {
	st := healthpb.HealthCheckResponse_NOT_SERVING
	if ok {
		st = healthpb.HealthCheckResponse_SERVING
	}
	if prev, seen := gs.last[service]; seen && prev == st {
		return
	}
	gs.last[service] = st
	gs.Server.SetServingStatus(service, st)
}
