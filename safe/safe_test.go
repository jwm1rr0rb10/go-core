package safe_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jwm1rr0rb10/go-core/safe"
)

func TestGoReturnsError(t *testing.T) {
	boom := errors.New("boom")
	err := <-safe.Go(context.Background(), func(context.Context) error { return boom }, nil)
	if err != boom {
		t.Fatalf("err=%v", err)
	}
}

func TestGoSuccessClosesChannel(t *testing.T) {
	errc := safe.Go(context.Background(), func(context.Context) error { return nil }, nil)
	if err, ok := <-errc; err != nil || ok {
		t.Fatalf("err=%v ok=%v", err, ok)
	}
}

func TestGoRecoversPanic(t *testing.T) {
	var got *safe.PanicError
	err := <-safe.Go(context.Background(), func(context.Context) error {
		panic("kaboom")
	}, func(p *safe.PanicError) { got = p })

	var pe *safe.PanicError
	if !errors.As(err, &pe) || pe.Value != "kaboom" || err.Error() != "panic: kaboom" {
		t.Fatalf("err=%v", err)
	}
	if got != pe {
		t.Fatal("recover handler must receive the same PanicError")
	}
	if !bytes.Contains(pe.Stack, []byte("safe_test.TestGoRecoversPanic")) {
		t.Fatalf("stack does not point at the panic site:\n%s", pe.Stack)
	}
}

func TestPanicWithErrorUnwraps(t *testing.T) {
	err := safe.Call(func() error { panic(io.EOF) }, safe.IgnoreRecover)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v", err)
	}
	if (&safe.PanicError{Value: 1}).Unwrap() != nil {
		t.Fatal("non-error value must unwrap to nil")
	}
}

func TestPanicNil(t *testing.T) {
	err := safe.Call(func() error { panic(nil) }, safe.IgnoreRecover)
	if _, ok := errors.AsType[*safe.PanicError](err); !ok {
		t.Fatalf("panic(nil) must be reported, got %v", err)
	}
}

func TestRecoverInDefer(t *testing.T) {
	work := func() (err error) {
		defer safe.Recover(&err, safe.IgnoreRecover)
		var m map[string]int
		m["x"] = 1 // runtime panic
		return nil
	}
	var pe *safe.PanicError
	if err := work(); !errors.As(err, &pe) {
		t.Fatalf("err=%v", err)
	}
}

func TestRecoverNilErrp(t *testing.T) {
	called := false
	func() {
		defer safe.Recover(nil, func(*safe.PanicError) { called = true })
		panic("x")
	}()
	if !called {
		t.Fatal("handler not called")
	}
}

func TestFuncAndCtxFunc(t *testing.T) {
	if err := safe.Func(func() error { return nil }, nil)(); err != nil {
		t.Fatal(err)
	}
	if err := safe.CtxFunc(func(ctx context.Context) error { return ctx.Err() }, nil)(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := safe.Func(func() error { panic("x") }, safe.IgnoreRecover)()
	if err == nil || err.Error() != "panic: x" {
		t.Fatalf("err=%v", err)
	}
}

func TestDeprecatedAliases(t *testing.T) {
	if err := <-safe.SafeGo(context.Background(), func(context.Context) error { panic("a") }, safe.IgnoreRecover); err == nil {
		t.Fatal("SafeGo")
	}
	if err := safe.SafeFunc(func() error { panic("b") }, safe.IgnoreRecover)(); err == nil {
		t.Fatal("SafeFunc")
	}
	if err := safe.SafeCtxFunc(func(context.Context) error { panic("c") }, safe.IgnoreRecover)(context.Background()); err == nil {
		t.Fatal("SafeCtxFunc")
	}
}

func TestDefaultRecoverDoesNotPanic(t *testing.T) {
	safe.DefaultRecover(safe.NewPanicError("x"))
}

func BenchmarkCallNoPanic(b *testing.B) {
	fn := func() error { return nil }
	for b.Loop() {
		_ = safe.Call(fn, nil)
	}
}

func BenchmarkCallPanic(b *testing.B) {
	fn := func() error { panic("x") }
	for b.Loop() {
		_ = safe.Call(fn, safe.IgnoreRecover)
	}
}

func BenchmarkGo(b *testing.B) {
	ctx := context.Background()
	fn := func(context.Context) error { return nil }
	for b.Loop() {
		<-safe.Go(ctx, fn, nil)
	}
}
