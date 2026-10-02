package closer_test

import (
	"context"
	"fmt"
	"time"

	"github.com/jwm1rr0rb10/go-core/closer"
)

func ExampleLIFOCloser() {
	lc := closer.NewLIFOCloser(
		closer.WithTimeout(10*time.Second),
		closer.WithCloserTimeout(3*time.Second),
	)
	lc.AddNamed("db", closer.CloserFunc(func() error {
		fmt.Println("db closed")
		return nil
	}))
	lc.AddFunc("http", func(ctx context.Context) error { // e.g. srv.Shutdown
		fmt.Println("http server stopped")
		return nil
	})

	fmt.Println(lc.Close())
	// Output:
	// http server stopped
	// db closed
	// <nil>
}

func ExampleCloseOnSignal() {
	lc := closer.NewLIFOCloser(closer.WithTimeout(15 * time.Second))
	// lc.Add(db); lc.AddFunc("http", srv.Shutdown) ...

	// In main, after starting servers:
	//	if err := closer.CloseOnSignal(lc); err != nil { ... }
	_ = lc
}
