package repeat

import "time"

// OptionSetter is the former name of [Option].
//
// Deprecated: use [Option].
type OptionSetter = Option

// WithMinWait sets the base delay.
//
// Deprecated: use [WithBaseDelay]. The old min/max range semantics are gone:
// delays now follow capped exponential backoff with jitter.
func WithMinWait(d time.Duration) Option { return WithBaseDelay(d) }

// WithMaxWait sets the maximum delay.
//
// Deprecated: use [WithMaxDelay].
func WithMaxWait(d time.Duration) Option { return WithMaxDelay(d) }

// WithExponentialBackoff sets base and max delay. Backoff is always on now.
//
// Deprecated: use [WithBackoff].
func WithExponentialBackoff(base, max time.Duration) Option { return WithBackoff(base, max) }

// WithErrorFilter retries only errors accepted by fn.
//
// Deprecated: use [WithRetryIf].
func WithErrorFilter(fn func(error) bool) Option { return WithRetryIf(fn) }
