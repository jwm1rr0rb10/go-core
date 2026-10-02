// Package closer releases application resources in reverse order of
// acquisition, typically during graceful shutdown.
//
// Register resources as they are opened — database pools, brokers, HTTP
// servers — and close them all with one call:
//
//	lc := closer.NewLIFOCloser(closer.WithTimeout(15 * time.Second))
//	lc.Add(db)                              // io.Closer
//	lc.AddFunc("http", srv.Shutdown)        // func(context.Context) error
//	lc.AddNoErr(producer)                   // Close() without error
//	err := closer.CloseOnSignal(lc)         // blocks until SIGINT/SIGTERM
//
// Every resource is closed exactly once, last registered first, even when
// some of them fail, panic or hang: failures are joined into one error,
// panics are recovered, and closers that exceed [WithCloserTimeout] or the
// overall [WithTimeout] are reported and left running in the background
// while shutdown continues. Close is idempotent; later calls return the
// first result.
package closer
