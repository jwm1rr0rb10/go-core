// Package repeat retries operations with capped exponential backoff and
// jitter.
//
// The zero-configuration call
//
//	err := repeat.Exec(ctx, op)
//
// makes at most [DefaultMaxAttempts] attempts, waiting
// [DefaultBaseDelay]·2ⁿ (capped at [DefaultMaxDelay]) between them with full
// jitter. Every knob is an [Option]; a [Retrier] built once with [New] can be
// shared by any number of goroutines and executes without per-call
// allocations.
//
// Retrying stops when:
//   - the operation returns nil;
//   - the error is marked with [Permanent] or rejected by [WithRetryIf] /
//     [WithRetryPolicy];
//   - the attempt budget ([WithMaxAttempts]) or time budget ([WithMaxElapsed])
//     is spent;
//   - the context is cancelled, or its deadline would pass before the next
//     attempt could start.
//
// When retries are exhausted the result is an [*Error] that carries the
// attempt count and unwraps to the last operation error (and to the context
// error or [ErrMaxElapsed] when that was the reason), so [errors.Is] and
// [errors.As] keep working.
//
// [Transport] and [NewClient] add retries to net/http, honoring Retry-After
// and rewinding request bodies; [ConnectWithRetry] does the same for
// gorilla/websocket dials.
package repeat
