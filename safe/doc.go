// Package safe turns panics into errors.
//
// A panic in a goroutine you started kills the whole process; a panic in a
// request handler kills it too unless something recovers it. The helpers in
// this package recover the panic, report it to a [RecoverFunc] (by default
// [DefaultRecover], which logs through slog.Default) and return it as a
// [*PanicError] that carries the panic value and the stack trace captured at
// the point of the panic.
//
//	errc := safe.Go(ctx, worker, nil)  // goroutine, result on a channel
//	err := safe.Call(fn, nil)          // run inline
//	defer safe.Recover(&err, nil)      // inside your own function
//
// The subpackages [github.com/jwm1rr0rb10/go-core/safe/errorgroup] and
// [github.com/jwm1rr0rb10/go-core/safe/waitgroup] build panic-safe goroutine
// groups on top of it.
package safe
