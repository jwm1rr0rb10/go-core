// Package clock abstracts the passage of time so that code depending on
// timers, tickers and timeouts can be tested deterministically.
//
// Production code takes a [Clock] (usually [New], backed by the time package)
// and tests pass a [Mock], whose time only moves when the test calls
// [Mock.Advance] or [Mock.Set]. The API mirrors the time package: durations
// of zero or less behave exactly as they do there (a timer fires
// immediately, NewTicker panics), and timer channels follow the Go 1.23+
// semantics where Stop and Reset never leave a stale value behind.
package clock
