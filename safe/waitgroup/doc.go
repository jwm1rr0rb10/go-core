// Package waitgroup provides a [sync.WaitGroup] that reports misuse instead
// of corrupting its state, exposes its counter, and can wait with a
// context or timeout.
//
// A plain sync.WaitGroup panics when its counter goes negative and gives no
// way to wait with a deadline. [WaitGroup] rejects such a call (returning
// an error, or panicking with [WithPanicOnMisuse]) and leaves the counter
// unchanged, so the group keeps working.
package waitgroup
