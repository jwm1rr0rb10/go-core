package clock_test

import (
	"fmt"
	"time"

	"github.com/jwm1rr0rb10/go-core/clock"
)

// retryAfter waits for d on the given clock and reports the time it woke up.
func retryAfter(c clock.Clock, d time.Duration) time.Time {
	c.Sleep(d)
	return c.Now()
}

func ExampleMock() {
	m := clock.NewMock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	done := make(chan time.Time)
	go func() { done <- retryAfter(m, 30*time.Second) }()

	m.BlockUntil(1) // the goroutine is now sleeping
	m.Advance(30 * time.Second)

	fmt.Println((<-done).Format(time.TimeOnly))
	// Output: 00:00:30
}

func ExampleMock_NewTicker() {
	m := clock.NewMock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	tk := m.NewTicker(time.Minute)
	defer tk.Stop()

	for range 3 {
		m.Advance(time.Minute)
		fmt.Println((<-tk.C()).Format(time.TimeOnly))
	}
	// Output:
	// 00:01:00
	// 00:02:00
	// 00:03:00
}

func ExampleNew() {
	c := clock.New()
	t := c.NewTimer(time.Millisecond)
	<-t.C()
	fmt.Println("fired")
	// Output: fired
}
