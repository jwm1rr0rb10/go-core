package errorgroup_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/jwm1rr0rb10/go-core/safe"
	"github.com/jwm1rr0rb10/go-core/safe/errorgroup"
)

func ExampleWithContext() {
	g, ctx := errorgroup.WithContext(context.Background(), errorgroup.WithLimit(4))
	for i := range 3 {
		g.Go(func(ctx context.Context) error {
			if i == 1 {
				return fmt.Errorf("task %d failed", i)
			}
			return nil
		})
	}
	fmt.Println(g.Wait())
	fmt.Println(ctx.Err())
	// Output:
	// task 1 failed
	// context canceled
}

func ExampleWithCollectAll() {
	g := errorgroup.New(errorgroup.WithCollectAll(), errorgroup.WithRecover(safe.IgnoreRecover))
	errA := errors.New("a failed")
	g.Go(func(context.Context) error { return errA })
	g.Go(func(context.Context) error { panic("b crashed") })

	err := g.Wait()
	var pe *safe.PanicError
	fmt.Println(errors.Is(err, errA), errors.As(err, &pe), len(g.Errors()))
	// Output: true true 2
}
