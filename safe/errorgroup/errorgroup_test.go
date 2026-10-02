package errorgroup_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/safe"
	"github.com/jwm1rr0rb10/go-core/safe/errorgroup"
)

func TestReturnsFirstErrorAndCancels(t *testing.T) {
	g, ctx := errorgroup.WithContext(context.Background())
	boom := errors.New("boom")
	g.Go(func(context.Context) error { return boom })
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	if err := g.Wait(); err != boom {
		t.Fatalf("err=%v", err)
	}
	if context.Cause(ctx) != boom {
		t.Fatalf("cause=%v", context.Cause(ctx))
	}
	if len(g.Errors()) != 1 {
		t.Fatalf("errors=%v", g.Errors())
	}
}

func TestPanicRecordedOnce(t *testing.T) {
	var handled atomic.Int32
	g, _ := errorgroup.WithContext(context.Background(),
		errorgroup.WithRecover(func(*safe.PanicError) { handled.Add(1) }))
	g.Go(func(context.Context) error { panic("kaboom") })
	err := g.Wait()
	var pe *safe.PanicError
	if !errors.As(err, &pe) || pe.Value != "kaboom" {
		t.Fatalf("err=%v", err)
	}
	if n := len(g.Errors()); n != 1 || handled.Load() != 1 {
		t.Fatalf("errors=%d handled=%d", n, handled.Load())
	}
}

func TestCollectAll(t *testing.T) {
	g, ctx := errorgroup.WithContext(context.Background(),
		errorgroup.WithCollectAll(), errorgroup.WithContinueOnError(), errorgroup.WithRecover(safe.IgnoreRecover))
	var finished atomic.Bool
	g.Go(func(context.Context) error { return io.EOF })
	g.Go(func(context.Context) error { panic(io.ErrClosedPipe) })
	g.Go(func(ctx context.Context) error {
		time.Sleep(20 * time.Millisecond)
		if ctx.Err() == nil {
			finished.Store(true)
		}
		return nil
	})
	err := g.Wait()
	if !errors.Is(err, io.EOF) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("err=%v", err)
	}
	if !finished.Load() {
		t.Fatal("context must stay alive with WithContinueOnError")
	}
	if ctx.Err() == nil {
		t.Fatal("context must be cancelled after Wait")
	}
}

func TestZeroValue(t *testing.T) {
	var g errorgroup.Group
	var n atomic.Int32
	for range 10 {
		g.Go(func(ctx context.Context) error {
			if ctx == nil {
				t.Error("nil ctx")
			}
			n.Add(1)
			return nil
		})
	}
	if err := g.Wait(); err != nil || n.Load() != 10 {
		t.Fatalf("err=%v n=%d", err, n.Load())
	}
}

func TestLimit(t *testing.T) {
	g := errorgroup.New(errorgroup.WithLimit(2))
	var active, peak atomic.Int32
	for range 20 {
		g.Go(func(context.Context) error {
			cur := active.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 2 {
		t.Fatalf("peak=%d", peak.Load())
	}
}

func TestTryGo(t *testing.T) {
	var g errorgroup.Group
	g.SetLimit(1)
	release := make(chan struct{})
	if !g.TryGo(func(context.Context) error { <-release; return nil }) {
		t.Fatal("first TryGo must start")
	}
	if g.TryGo(func(context.Context) error { return nil }) {
		t.Fatal("second TryGo must be rejected")
	}
	close(release)
	_ = g.Wait()
	if !g.TryGo(func(context.Context) error { return nil }) {
		t.Fatal("TryGo after slot freed must start")
	}
	_ = g.Wait()
	g.SetLimit(-1)
}

func TestSetLimitWhileActivePanics(t *testing.T) {
	var g errorgroup.Group
	g.SetLimit(1)
	release := make(chan struct{})
	g.Go(func(context.Context) error { <-release; return nil })
	defer func() {
		close(release)
		_ = g.Wait()
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	g.SetLimit(3)
}

func BenchmarkGroup(b *testing.B) {
	fn := func(context.Context) error { return nil }
	for b.Loop() {
		g, _ := errorgroup.WithContext(context.Background())
		for range 8 {
			g.Go(fn)
		}
		_ = g.Wait()
	}
}
