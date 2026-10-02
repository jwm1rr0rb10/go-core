package time_test

import (
	"fmt"
	"time"

	coretime "github.com/jwm1rr0rb10/go-core/time"
)

func ExampleUnixToTime() {
	sec := coretime.UnixToTime(1_767_225_600)    // seconds
	ms := coretime.UnixToTime(1_767_225_600_000) // milliseconds
	fmt.Println(sec.UTC().Format(time.DateOnly), sec.Equal(ms))
	// Output: 2026-01-01 true
}

func ExampleFormatDuration() {
	fmt.Println(coretime.FormatDuration(1234 * time.Microsecond))
	fmt.Println(coretime.FormatDuration(2500 * time.Millisecond))
	fmt.Println(coretime.FormatDuration(90 * time.Minute))
	// Output:
	// 1ms
	// 2.50s
	// 1h30m0s
}

func ExampleFormatWithTimezone() {
	t := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	s, _ := coretime.FormatWithTimezone(t, time.DateTime, "Asia/Tokyo")
	fmt.Println(s)
	// Output: 2026-07-11 21:00:00
}

func ExampleLocationByOffsetMinutes() {
	loc, _ := coretime.LocationByOffsetMinutes(5*60 + 45) // Nepal
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).In(loc)
	fmt.Println(loc, t.Format(time.TimeOnly))
	// Output: UTC+5:45 05:45:00
}
