// Package tcp provides production-oriented building blocks for TCP services:
//
//   - [Server]: an accept loop with connection limits, accept-error backoff,
//     idle timeouts, middleware, panic recovery, lock-free statistics and
//     context-driven graceful shutdown.
//   - [Client]: a context-aware client with optional TLS, buffered reads,
//     length-prefixed framing, automatic reconnect and retry with
//     exponential backoff and full jitter.
//   - [Pool]: a client pool with max-active limiting, idle/lifetime eviction
//     and a real (MSG_PEEK based) liveness check.
//   - [RateLimiter]: a sharded per-IP token-bucket limiter with temporary bans
//     and adaptive proof-of-work challenges, usable as server [Middleware].
//   - Proof of work: [NewPoWChallenge], [SolvePoW], [AnswerPoW] and the line
//     based wire protocol used by the rate limiter.
//   - Framing: [WriteFrame] and [ReadFrame] for 4-byte big-endian
//     length-prefixed messages.
//
// All types are safe for concurrent use unless documented otherwise. Nothing
// in this package logs on the hot path; pass an *slog.Logger through the
// options if you want diagnostics.
package tcp
