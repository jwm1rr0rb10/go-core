package healthcheck_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jwm1rr0rb10/go-core/healthcheck"
)

const (
	serving    = healthpb.HealthCheckResponse_SERVING
	notServing = healthpb.HealthCheckResponse_NOT_SERVING
)

func statusOf(t *testing.T, hs *healthcheck.GRPCHealthServer, service string) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := hs.Check(context.Background(), &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		t.Fatalf("check %q: %v", service, err)
	}
	return resp.GetStatus()
}

func waitStatus(t *testing.T, hs *healthcheck.GRPCHealthServer, service string, want healthpb.HealthCheckResponse_ServingStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := hs.Check(context.Background(), &healthpb.HealthCheckRequest{Service: service}); err == nil && resp.GetStatus() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("service %q: expected %v", service, want)
}

func TestServingWithoutDependencies(t *testing.T) {
	if got := statusOf(t, healthcheck.NewGRPCHealthServer(), ""); got != serving {
		t.Fatalf("got %v", got)
	}
}

func TestSetStatusIsImmediate(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer()
	hs.SetStatus("db", true)
	hs.SetStatus("cache", true)
	if got := statusOf(t, hs, ""); got != serving {
		t.Fatalf("got %v", got)
	}
	hs.SetStatus("db", false)
	if got := statusOf(t, hs, ""); got != notServing {
		t.Fatalf("got %v", got)
	}
	if err := hs.Errors()["db"]; err == nil {
		t.Fatal("expected error for db")
	}
	hs.RemoveDependency("db")
	if got := statusOf(t, hs, ""); got != serving {
		t.Fatalf("after remove: %v", got)
	}
	if s := hs.Statuses(); len(s) != 1 || !s["cache"] {
		t.Fatalf("statuses %v", s)
	}
}

func TestBindService(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer()
	hs.SetStatus("db", true)
	hs.SetStatus("search", false)
	hs.BindService("orders.Orders", "db")
	hs.BindService("catalog.Catalog", "db", "search")
	hs.BindService("ghost.Ghost", "unknown")

	if statusOf(t, hs, "orders.Orders") != serving {
		t.Fatal("orders should serve")
	}
	if statusOf(t, hs, "catalog.Catalog") != notServing {
		t.Fatal("catalog should not serve")
	}
	if statusOf(t, hs, "ghost.Ghost") != notServing {
		t.Fatal("unknown dependency must count as unhealthy")
	}
	if statusOf(t, hs, "") != notServing {
		t.Fatal("overall must reflect search")
	}
	hs.SetStatus("search", true)
	if statusOf(t, hs, "catalog.Catalog") != serving {
		t.Fatal("catalog should recover")
	}
}

func TestCheckersArePolled(t *testing.T) {
	var healthy atomic.Bool
	hs := healthcheck.NewGRPCHealthServer(healthcheck.WithInterval(10 * time.Millisecond))
	hs.AddChecker("db", func(context.Context) error {
		if healthy.Load() {
			return nil
		}
		return errors.New("down")
	})
	if statusOf(t, hs, "") != notServing {
		t.Fatal("unchecked dependency must be unhealthy")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hs.Start(ctx)
	hs.Start(ctx) // idempotent

	healthy.Store(true)
	waitStatus(t, hs, "", serving)
	healthy.Store(false)
	waitStatus(t, hs, "", notServing)
	hs.Stop()
	hs.Stop() // idempotent
}

func TestCheckerTimeoutAndPanic(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer(healthcheck.WithCheckTimeout(10 * time.Millisecond))
	hs.AddChecker("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	hs.AddChecker("panics", func(context.Context) error { panic("boom") })

	hs.CheckNow(context.Background())
	errs := hs.Errors()
	if !errors.Is(errs["slow"], context.DeadlineExceeded) {
		t.Fatalf("slow: %v", errs["slow"])
	}
	if errs["panics"] == nil {
		t.Fatal("panic must be reported as error")
	}
}

func TestShutdownAndResume(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer()
	hs.SetStatus("db", true)
	hs.Start(context.Background())
	hs.Shutdown()
	if statusOf(t, hs, "") != notServing {
		t.Fatal("shutdown must report NOT_SERVING")
	}
	hs.SetStatus("db", true)
	if statusOf(t, hs, "") != notServing {
		t.Fatal("updates after shutdown must be ignored")
	}
	hs.Resume()
	if statusOf(t, hs, "") != serving {
		t.Fatal("resume must restore status")
	}
}

func TestStopOnContextCancel(t *testing.T) {
	var calls atomic.Int32
	hs := healthcheck.NewGRPCHealthServer(healthcheck.WithInterval(5 * time.Millisecond))
	hs.AddChecker("x", func(context.Context) error { calls.Add(1); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	hs.Start(ctx)
	waitStatus(t, hs, "", serving)
	cancel()
	hs.Stop()
	n := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != n {
		t.Fatal("checker polled after stop")
	}
	// The server can be started again after the loop exited.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	hs.Start(ctx2)
	deadline := time.Now().Add(time.Second)
	for calls.Load() == n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() == n {
		t.Fatal("restart did not poll")
	}
	hs.Stop()
}

func TestConcurrentUse(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer(healthcheck.WithInterval(time.Millisecond))
	hs.AddChecker("c", func(context.Context) error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hs.Start(ctx)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				hs.SetStatus("dep", (i+j)%2 == 0)
				_ = hs.Statuses()
				_, _ = hs.Check(context.Background(), &healthpb.HealthCheckRequest{})
			}
		})
	}
	wg.Wait()
	hs.Shutdown()
}

func TestDeprecatedHealthCheck(t *testing.T) {
	hs := healthcheck.NewGRPCHealthServer()
	hs.SetStatus("db", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hs.HealthCheck(ctx, 10*time.Millisecond)
	waitStatus(t, hs, "", serving)
	hs.Stop()
}

func BenchmarkSetStatus(b *testing.B) {
	hs := healthcheck.NewGRPCHealthServer()
	for i := range 20 {
		hs.SetStatus(string(rune('a'+i)), true)
	}
	for b.Loop() {
		hs.SetStatus("a", true)
	}
}

func BenchmarkCheck(b *testing.B) {
	hs := healthcheck.NewGRPCHealthServer()
	req := &healthpb.HealthCheckRequest{}
	for b.Loop() {
		_, _ = hs.Check(context.Background(), req)
	}
}
