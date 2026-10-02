package waitgroup_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/safe/waitgroup"
)

func TestWaitsForTasks(t *testing.T) {
	wg := waitgroup.NewWaitGroup()
	var hits atomic.Int32
	for range 50 {
		if err := wg.Add(1); err != nil {
			t.Fatal(err)
		}
		go func() {
			defer func() { _ = wg.Done() }()
			hits.Add(1)
		}()
	}
	wg.Wait()
	if hits.Load() != 50 || wg.Count() != 0 {
		t.Fatalf("hits=%d count=%d", hits.Load(), wg.Count())
	}
}

func TestNegativeCounterRejectedAndStateKept(t *testing.T) {
	var wg waitgroup.WaitGroup
	if err := wg.Done(); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("err=%v", err)
	}
	if wg.Count() != 0 {
		t.Fatalf("count=%d", wg.Count())
	}
	// Still usable after misuse.
	_ = wg.Add(2)
	if err := wg.Add(-3); err == nil || wg.Count() != 2 {
		t.Fatalf("err=%v count=%d", err, wg.Count())
	}
	_ = wg.Done()
	_ = wg.Done()
	if !wg.WaitTimeout(time.Second) {
		t.Fatal("wait should complete")
	}
}

func TestPanicsOnMisuseWhenConfigured(t *testing.T) {
	wg := waitgroup.NewWaitGroup(waitgroup.WithPanicOnMisuse())
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = wg.Done()
}

func TestOverflowRejected(t *testing.T) {
	var wg waitgroup.WaitGroup
	if err := wg.Add(1 << 31); err == nil || wg.Count() != 0 {
		t.Fatalf("err=%v count=%d", err, wg.Count())
	}
	if err := wg.Add(0); err != nil {
		t.Fatal(err)
	}
}

func TestGo(t *testing.T) {
	var wg waitgroup.WaitGroup
	var n atomic.Int32
	for range 100 {
		wg.Go(func() { n.Add(1) })
	}
	wg.Wait()
	if n.Load() != 100 {
		t.Fatalf("n=%d", n.Load())
	}
}

func TestWaitContext(t *testing.T) {
	var wg waitgroup.WaitGroup
	if err := wg.WaitContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	wg.Go(func() { <-release })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := wg.WaitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if wg.WaitTimeout(time.Millisecond) {
		t.Fatal("must time out")
	}
	close(release)
	if !wg.WaitTimeout(time.Second) {
		t.Fatal("must finish")
	}
}

func TestConcurrentAddDone(t *testing.T) {
	var wg waitgroup.WaitGroup
	var outer waitgroup.WaitGroup
	for range 8 {
		outer.Go(func() {
			for range 1000 {
				_ = wg.Add(1)
				go func() { _ = wg.Done() }()
			}
		})
	}
	outer.Wait()
	wg.Wait()
	if wg.Count() != 0 {
		t.Fatalf("count=%d", wg.Count())
	}
}

func BenchmarkAddDone(b *testing.B) {
	var wg waitgroup.WaitGroup
	for b.Loop() {
		_ = wg.Add(1)
		_ = wg.Done()
	}
}

func BenchmarkAddDoneParallel(b *testing.B) {
	var wg waitgroup.WaitGroup
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = wg.Add(1)
			_ = wg.Done()
		}
	})
}
