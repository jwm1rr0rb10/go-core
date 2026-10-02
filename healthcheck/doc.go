// Package healthcheck provides a gRPC health server (grpc.health.v1) whose
// status is derived from the health of the service's dependencies.
//
// Dependencies are reported either push-style with
// [GRPCHealthServer.SetStatus] or pull-style by registering a [CheckFunc] with
// [GRPCHealthServer.AddChecker], which is polled periodically after
// [GRPCHealthServer.Start]. The overall status ("" service) is SERVING only
// when every dependency is healthy; named services can be bound to a subset
// of dependencies with [GRPCHealthServer.BindService].
//
// All methods are safe for concurrent use.
package healthcheck
