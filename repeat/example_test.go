package repeat_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jwm1rr0rb10/go-core/repeat"
)

func ExampleExec() {
	err := repeat.Exec(context.Background(), func(_ context.Context, attempt int) error {
		if attempt < 2 {
			return errors.New("temporary failure")
		}
		fmt.Println("succeeded on attempt", attempt)
		return nil
	}, repeat.WithBackoff(time.Millisecond, 10*time.Millisecond))
	fmt.Println("err:", err)
	// Output:
	// succeeded on attempt 2
	// err: <nil>
}

func ExampleDo() {
	n, err := repeat.Do(context.Background(), func(context.Context) (int, error) {
		return 42, nil
	})
	fmt.Println(n, err)
	// Output: 42 <nil>
}

func ExamplePermanent() {
	errNotFound := errors.New("not found")
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return repeat.Permanent(errNotFound)
	})
	fmt.Println(calls, err == errNotFound)
	// Output: 1 true
}

func ExampleNew() {
	// Build once, share between goroutines.
	retrier, err := repeat.New(
		repeat.WithMaxAttempts(5),
		repeat.WithBackoff(50*time.Millisecond, 2*time.Second),
		repeat.WithJitter(repeat.JitterEqual),
		repeat.WithMaxElapsed(10*time.Second),
		repeat.WithOnRetry(func(e repeat.RetryEvent) {
			// metrics.Retries.Inc(); log.Debug(...)
		}),
	)
	if err != nil {
		panic(err)
	}
	err = retrier.Exec(context.Background(), func(context.Context, int) error { return nil })
	fmt.Println(err)
	// Output: <nil>
}

func ExampleError() {
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		return errors.New("boom")
	}, repeat.WithConstantDelay(time.Millisecond), repeat.WithMaxAttempts(3))

	var re *repeat.Error
	if errors.As(err, &re) {
		fmt.Println(re.Attempts, re.Err)
	}
	// Output: 3 boom
}

func ExampleNewClient() {
	client := repeat.NewClient(&http.Client{Timeout: 5 * time.Second},
		repeat.WithMaxAttempts(3),
		repeat.WithBackoff(100*time.Millisecond, time.Second),
	)
	_ = client // use like any *http.Client: client.Get(url), client.Do(req)
}
