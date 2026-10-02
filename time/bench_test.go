package time_test

import (
	"testing"
	"time"

	coretime "github.com/jwm1rr0rb10/go-core/time"
)

func BenchmarkStdLoadLocation(b *testing.B) {
	for b.Loop() {
		if _, err := time.LoadLocation("Europe/Moscow"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadLocationCached(b *testing.B) {
	for b.Loop() {
		if _, err := coretime.LoadLocation("Europe/Moscow"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnixToTime(b *testing.B) {
	ts := time.Now().UnixMilli()
	for b.Loop() {
		_ = coretime.UnixToTime(ts)
	}
}

func BenchmarkFormatDuration(b *testing.B) {
	for b.Loop() {
		_ = coretime.FormatDuration(1500 * time.Millisecond)
	}
}
