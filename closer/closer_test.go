package closer_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/closer"
	"github.com/jwm1rr0rb10/go-core/safe"
)

type recorder struct {
	mu    sync.Mutex
	order []string
}

func (r *recorder) push(s string) {
	r.mu.Lock()
	r.order = append(r.order, s)
	r.mu.Unlock()
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.order, ",")
}

func TestLIFOAcrossKinds(t *testing.T) {
	var rec recorder
	lc := closer.NewLIFOCloser()
	lc.Add(closer.CloserFunc(func() error { rec.push("a"); return nil }))
	lc.AddNoErr(closer.NoErrCloserFunc(func() { rec.push("b") }))
	lc.AddFunc("c", func(context.Context) error { rec.push("c"); return nil })
	lc.AddNamed("d", closer.CloserFunc(func() error { rec.push("d"); return nil }))
	if lc.Len() != 4 {
		t.Fatalf("len=%d", lc.Len())
	}
	if err := lc.Close(); err != nil {
		t.Fatal(err)
	}
	if got := rec.String(); got != "d,c,b,a" {
		t.Fatalf("order=%s", got)
	}
}

func TestCloseIdempotent(t *testing.T) {
	var calls atomic.Int32
	lc := closer.NewLIFOCloser()
	lc.AddNamed("db", closer.CloserFunc(func() error { calls.Add(1); return io.ErrClosedPipe }))
	first := lc.Close()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if err := lc.Close(); err != first {
				t.Errorf("err=%v want %v", err, first)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 || !errors.Is(first, io.ErrClosedPipe) {
		t.Fatalf("calls=%d err=%v", calls.Load(), first)
	}
	if !strings.Contains(first.Error(), "close db:") {
		t.Fatalf("err must name the closer: %v", first)
	}
}

func TestJoinsErrorsAndContinues(t *testing.T) {
	var rec recorder
	lc := closer.NewLIFOCloser()
	lc.AddNamed("first", closer.CloserFunc(func() error { rec.push("1"); return io.EOF }))
	lc.AddNamed("second", closer.CloserFunc(func() error { rec.push("2"); return io.ErrUnexpectedEOF }))
	err := lc.Close()
	if !errors.Is(err, io.EOF) || !errors.Is(err, io.ErrUnexpectedEOF) || rec.String() != "2,1" {
		t.Fatalf("err=%v order=%s", err, rec.String())
	}
}

func TestPanicRecovered(t *testing.T) {
	var rec recorder
	lc := closer.NewLIFOCloser()
	lc.AddNoErr(closer.NoErrCloserFunc(func() { rec.push("after") }))
	lc.AddNamed("bad", closer.CloserFunc(func() error { panic("boom") }))
	err := lc.Close()
	var pe *safe.PanicError
	if !errors.As(err, &pe) || rec.String() != "after" {
		t.Fatalf("err=%v order=%s", err, rec.String())
	}
}

func TestCloserTimeout(t *testing.T) {
	var rec recorder
	release := make(chan struct{})
	defer close(release)
	lc := closer.NewLIFOCloser(closer.WithCloserTimeout(20 * time.Millisecond))
	lc.AddNoErr(closer.NoErrCloserFunc(func() { rec.push("next") }))
	lc.AddNamed("hung", closer.CloserFunc(func() error { <-release; return nil }))
	start := time.Now()
	err := lc.Close()
	if !errors.Is(err, context.DeadlineExceeded) || rec.String() != "next" {
		t.Fatalf("err=%v order=%s", err, rec.String())
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout not applied")
	}
}

func TestOverallTimeoutPassesContext(t *testing.T) {
	lc := closer.NewLIFOCloser(closer.WithTimeout(20 * time.Millisecond))
	var hadDeadline atomic.Bool
	lc.AddFunc("srv", func(ctx context.Context) error {
		_, ok := ctx.Deadline()
		hadDeadline.Store(ok)
		<-ctx.Done()
		return ctx.Err()
	})
	err := lc.Close()
	if !errors.Is(err, context.DeadlineExceeded) || !hadDeadline.Load() {
		t.Fatalf("err=%v deadline=%v", err, hadDeadline.Load())
	}
}

func TestAddAfterCloseClosesImmediately(t *testing.T) {
	lc := closer.NewLIFOCloser()
	_ = lc.Close()
	var closed atomic.Bool
	lc.AddNoErr(closer.NoErrCloserFunc(func() { closed.Store(true) }))
	if !closed.Load() || lc.Len() != 0 {
		t.Fatal("late resource must be closed immediately")
	}
}

func TestAddDuringClose(t *testing.T) {
	lc := closer.NewLIFOCloser()
	var late atomic.Bool
	lc.AddFunc("spawner", func(context.Context) error {
		lc.AddNoErr(closer.NoErrCloserFunc(func() { late.Store(true) }))
		return nil
	})
	if err := lc.Close(); err != nil || !late.Load() {
		t.Fatalf("err=%v late=%v", err, late.Load())
	}
}

func TestZeroValueAndNils(t *testing.T) {
	var lc closer.LIFOCloser
	lc.Add(nil)
	lc.AddNoErr(nil)
	lc.AddFunc("nil", nil)
	if err := lc.Close(); err != nil || lc.Len() != 0 {
		t.Fatal(err)
	}
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	lc := closer.NewLIFOCloser(closer.WithLogger(slog.New(slog.NewTextHandler(&buf, nil))))
	lc.AddNamed("db", closer.CloserFunc(func() error { return io.EOF }))
	_ = lc.Close()
	if !strings.Contains(buf.String(), "name=db") {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestCloseOnSignalWithContext(t *testing.T) {
	lc := closer.NewLIFOCloser()
	var closed atomic.Bool
	lc.AddNoErr(closer.NoErrCloserFunc(func() { closed.Store(true) }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := closer.CloseOnSignalWithContext(ctx, lc); err != nil || !closed.Load() {
		t.Fatalf("err=%v closed=%v", err, closed.Load())
	}
}

func TestCloseOnSignalContext(t *testing.T) {
	lc := closer.NewLIFOCloser()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := closer.CloseOnSignalContext(lc)(ctx); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkAddClose(b *testing.B) {
	c := closer.CloserFunc(func() error { return nil })
	for b.Loop() {
		lc := closer.NewLIFOCloser()
		for range 8 {
			lc.AddNamed("r", c)
		}
		_ = lc.Close()
	}
}
