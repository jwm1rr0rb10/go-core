package safe_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/jwm1rr0rb10/go-core/safe"
)

func ExampleGo() {
	errc := safe.Go(context.Background(), func(context.Context) error {
		panic("worker crashed")
	}, safe.IgnoreRecover)

	err := <-errc
	var pe *safe.PanicError
	fmt.Println(errors.As(err, &pe), pe.Value)
	// Output: true worker crashed
}

func ExampleCall() {
	err := safe.Call(func() error {
		var s []int
		_ = s[3] // index out of range
		return nil
	}, safe.IgnoreRecover)
	fmt.Println(err)
	// Output: panic: runtime error: index out of range [3] with length 0
}

func ExampleRecover() {
	parse := func() (err error) {
		defer safe.Recover(&err, safe.IgnoreRecover)
		panic("bad input")
	}
	fmt.Println(parse())
	// Output: panic: bad input
}
