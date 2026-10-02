// Package errorgroup runs a group of goroutines, recovers their panics and
// collects their errors.
//
// It follows the API of golang.org/x/sync/errgroup (Go, TryGo, SetLimit,
// Wait, WithContext) and adds:
//
//   - panic recovery: a panic in one task becomes a
//     [*github.com/jwm1rr0rb10/go-core/safe.PanicError] instead of crashing
//     the process;
//   - error collection: [Group.Errors] returns every error, and with
//     [WithCollectAll] Wait returns all of them joined (errors.Is and
//     errors.As see each one);
//   - control over cancellation: by default the context returned by
//     [WithContext] is cancelled on the first error, with that error as its
//     cause (context.Cause); [WithContinueOnError] keeps it alive so the other
//     tasks run to completion.
//
// The derived context is always cancelled when Wait returns.
package errorgroup
