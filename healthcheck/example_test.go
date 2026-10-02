package healthcheck_test

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jwm1rr0rb10/go-core/healthcheck"
)

func Example() {
	hs := healthcheck.NewGRPCHealthServer(
		healthcheck.WithInterval(5*time.Second),
		healthcheck.WithCheckTimeout(time.Second),
	)

	// Pull-style: polled by Start.
	hs.AddChecker("postgres", func(ctx context.Context) error {
		return nil // e.g. db.PingContext(ctx)
	})
	// Push-style: e.g. from a NATS connection callback.
	hs.SetStatus("nats", true)
	// "orders.Orders" only depends on postgres.
	hs.BindService("orders.Orders", "postgres")

	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, hs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hs.Start(ctx)
	defer hs.Shutdown() // NOT_SERVING first, so balancers drain traffic

	hs.CheckNow(ctx)
	resp, _ := hs.Check(ctx, &healthpb.HealthCheckRequest{Service: "orders.Orders"})
	fmt.Println(resp.GetStatus())
	// Output: SERVING
}
