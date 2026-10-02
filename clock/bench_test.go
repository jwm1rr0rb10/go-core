package clock_test

import (
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/clock"
)

func BenchmarkRealNow(b *testing.B) {
	c := clock.New()
	for b.Loop() {
		_ = c.Now()
	}
}

func BenchmarkMockTimerAdvance(b *testing.B) {
	m := clock.NewMock(time.Time{})
	for b.Loop() {
		t := m.NewTimer(time.Second)
		m.Advance(time.Second)
		<-t.C()
	}
}
