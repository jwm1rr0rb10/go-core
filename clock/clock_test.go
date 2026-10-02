package clock_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/clock"
)

var start = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

func TestRealClock(t *testing.T) {
	c := clock.New()
	before := time.Now()
	if c.Now().Before(before) {
		t.Fatal("Now is before time.Now")
	}
	if c.Since(before.Add(-time.Second)) < time.Second {
		t.Fatal("Since too small")
	}
	if c.Until(time.Now().Add(time.Hour)) <= 0 {
		t.Fatal("Until should be positive")
	}

	c.Sleep(0)
	c.Sleep(-time.Second)

	select {
	case <-c.After(0):
	case <-time.After(time.Second):
		t.Fatal("After(0) must fire immediately")
	}

	tm := c.NewTimer(time.Hour)
	if !tm.Stop() {
		t.Fatal("Stop on active timer should return true")
	}
	tm.Reset(time.Millisecond)
	<-tm.C()

	done := make(chan struct{})
	af := c.AfterFunc(time.Millisecond, func() { close(done) })
	<-done
	if af.C() != nil {
		t.Fatal("AfterFunc timer must have nil channel")
	}

	tk := c.NewTicker(time.Millisecond)
	<-tk.C()
	tk.Reset(2 * time.Millisecond)
	<-tk.C()
	tk.Stop()
}

func TestRealSleepContext(t *testing.T) {
	c := clock.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.SleepContext(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("want Canceled, got %v", err)
	}
	if err := c.SleepContext(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMockNowAdvanceSet(t *testing.T) {
	m := clock.NewMock(start)
	if !m.Now().Equal(start) {
		t.Fatal("wrong start")
	}
	m.Advance(2 * time.Hour)
	if got := m.Since(start); got != 2*time.Hour {
		t.Fatalf("Since = %v", got)
	}
	if got := m.Until(start); got != -2*time.Hour {
		t.Fatalf("Until = %v", got)
	}
	m.Advance(-time.Hour) // ignored
	if got := m.Since(start); got != 2*time.Hour {
		t.Fatalf("negative Advance moved the clock: %v", got)
	}
	m.Set(start) // backwards
	if !m.Now().Equal(start) {
		t.Fatal("Set backwards failed")
	}
}

func TestMockTimerFiresOnlyAfterDeadline(t *testing.T) {
	m := clock.NewMock(start)
	tm := m.NewTimer(10 * time.Second)

	m.Advance(9 * time.Second)
	select {
	case <-tm.C():
		t.Fatal("fired too early")
	default:
	}

	m.Advance(time.Second)
	select {
	case got := <-tm.C():
		if !got.Equal(start.Add(10 * time.Second)) {
			t.Fatalf("fired with %v", got)
		}
	default:
		t.Fatal("timer did not fire")
	}
	if tm.Stop() {
		t.Fatal("Stop after fire should return false")
	}
}

func TestMockTimerZeroDurationFiresImmediately(t *testing.T) {
	m := clock.NewMock(start)
	select {
	case <-m.After(0):
	default:
		t.Fatal("After(0) should fire immediately")
	}
}

func TestMockTimerStopAndReset(t *testing.T) {
	m := clock.NewMock(start)
	tm := m.NewTimer(time.Second)
	if !tm.Stop() {
		t.Fatal("Stop should report active")
	}
	m.Advance(time.Hour)
	select {
	case <-tm.C():
		t.Fatal("stopped timer fired")
	default:
	}

	if tm.Reset(time.Minute) {
		t.Fatal("Reset of stopped timer should return false")
	}
	m.Advance(time.Minute)
	select {
	case <-tm.C():
	default:
		t.Fatal("reset timer did not fire")
	}
}

func TestMockResetDropsStaleValue(t *testing.T) {
	m := clock.NewMock(start)
	tm := m.NewTimer(time.Second)
	m.Advance(time.Second) // value now buffered
	tm.Reset(time.Second)
	select {
	case <-tm.C():
		t.Fatal("stale value survived Reset")
	default:
	}
}

func TestMockFiresInDeadlineOrder(t *testing.T) {
	m := clock.NewMock(start)
	var order []int
	var seen []time.Time
	for _, d := range []int{3, 1, 2} {
		m.AfterFunc(time.Duration(d)*time.Second, func() {
			order = append(order, d)
			seen = append(seen, m.Now())
		})
	}
	m.Advance(5 * time.Second)

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order = %v", order)
	}
	for i, ts := range seen {
		if want := start.Add(time.Duration(i+1) * time.Second); !ts.Equal(want) {
			t.Fatalf("callback %d saw Now=%v, want %v", i, ts, want)
		}
	}
	if !m.Now().Equal(start.Add(5 * time.Second)) {
		t.Fatalf("final Now = %v", m.Now())
	}
}

func TestMockTicker(t *testing.T) {
	m := clock.NewMock(start)
	tk := m.NewTicker(time.Second)

	for i := 1; i <= 3; i++ {
		m.Advance(time.Second)
		got := <-tk.C()
		if want := start.Add(time.Duration(i) * time.Second); !got.Equal(want) {
			t.Fatalf("tick %d = %v, want %v", i, got, want)
		}
	}

	// A slow receiver keeps only one pending tick, like time.Ticker.
	m.Advance(5 * time.Second)
	<-tk.C()
	select {
	case <-tk.C():
		t.Fatal("ticks should not queue up")
	default:
	}

	tk.Reset(10 * time.Second)
	m.Advance(9 * time.Second)
	select {
	case <-tk.C():
		t.Fatal("reset ticker fired early")
	default:
	}
	m.Advance(time.Second)
	<-tk.C()

	tk.Stop()
	m.Advance(time.Hour)
	select {
	case <-tk.C():
		t.Fatal("stopped ticker ticked")
	default:
	}
	if m.Waiters() != 0 {
		t.Fatalf("waiters = %d", m.Waiters())
	}
}

func TestMockTickerPanicsOnNonPositive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	clock.NewMock(start).NewTicker(0)
}

func TestMockSleepBlocksUntilAdvance(t *testing.T) {
	m := clock.NewMock(start)
	var woke atomic.Bool
	done := make(chan struct{})
	go func() {
		m.Sleep(time.Minute)
		woke.Store(true)
		close(done)
	}()

	m.BlockUntil(1)
	if woke.Load() {
		t.Fatal("Sleep returned before Advance")
	}
	m.Advance(time.Minute)
	<-done
}

func TestMockSleepContextCancel(t *testing.T) {
	m := clock.NewMock(start)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- m.SleepContext(ctx, time.Hour) }()

	m.BlockUntil(1)
	cancel()
	if err := <-errc; err != context.Canceled {
		t.Fatalf("want Canceled, got %v", err)
	}
	if n := m.Waiters(); n != 0 {
		t.Fatalf("cancelled sleep leaked a waiter: %d", n)
	}
	if err := m.SleepContext(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
}

func TestMockBlockUntilContext(t *testing.T) {
	m := clock.NewMock(start)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := m.BlockUntilContext(ctx, 1); err != context.DeadlineExceeded {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

func TestMockSetForwardFires(t *testing.T) {
	m := clock.NewMock(start)
	ch := m.After(time.Hour)
	m.Set(start.Add(2 * time.Hour))
	select {
	case <-ch:
	default:
		t.Fatal("Set forward did not fire timer")
	}
}

func TestMockConcurrentUse(t *testing.T) {
	m := clock.NewMock(start)
	var wg sync.WaitGroup
	const sleepers = 50
	for range sleepers {
		wg.Go(func() { m.Sleep(time.Second) })
	}
	m.BlockUntil(sleepers)
	m.Advance(time.Second)
	wg.Wait()
}

// Compile-time check that the mock satisfies Clock.
var _ clock.Clock = clock.NewMock(time.Time{})
