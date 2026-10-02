package waitgroup_test

import (
	"fmt"
	"time"

	"github.com/jwm1rr0rb10/go-core/safe/waitgroup"
)

func ExampleWaitGroup() {
	var wg waitgroup.WaitGroup
	for range 3 {
		wg.Go(func() { time.Sleep(time.Millisecond) })
	}
	fmt.Println("finished in time:", wg.WaitTimeout(time.Second))

	err := wg.Done() // one Done too many: rejected, state unchanged
	fmt.Println(err != nil, wg.Count())
	// Output:
	// finished in time: true
	// true 0
}
