package repeat_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jwm1rr0rb10/go-core/repeat"
)

var errTemp = errors.New("temporary")

func fast() repeat.Option { return repeat.WithConstantDelay(time.Millisecond) }

func TestExecSucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestExecRetriesUntilSuccess(t *testing.T) {
	var seen []int
	err := repeat.Exec(context.Background(), func(_ context.Context, attempt int) error {
		seen = append(seen, attempt)
		if attempt < 2 {
			return errTemp
		}
		return nil
	}, fast(), repeat.WithMaxAttempts(5))
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[0] != 0 || seen[2] != 2 {
		t.Fatalf("attempts: %v", seen)
	}
}

func TestExecMaxAttemptsIsTotal(t *testing.T) {
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return io.EOF
	}, fast(), repeat.WithMaxAttempts(3))
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
	var re *repeat.Error
	if !errors.As(err, &re) || re.Attempts != 3 || re.Cause != nil {
		t.Fatalf("unexpected error %#v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatal("errors.Is must reach the last error")
	}
}

func TestExecDefaultIsFinite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		_ = repeat.Exec(context.Background(), func(context.Context, int) error {
			calls++
			return errTemp
		})
		if calls != repeat.DefaultMaxAttempts {
			t.Fatalf("calls=%d", calls)
		}
	})
}

func TestExecSingleAttemptNoRetry(t *testing.T) {
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return errTemp
	}, repeat.WithMaxAttempts(1))
	if calls != 1 || !errors.Is(err, errTemp) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestExecPermanentStopsImmediately(t *testing.T) {
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return repeat.Permanent(errTemp)
	}, fast())
	if calls != 1 || err != errTemp {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if repeat.Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must be nil")
	}
}

func TestExecWrappedPermanent(t *testing.T) {
	wrapped := errors.Join(repeat.Permanent(errTemp), io.EOF)
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return wrapped
	}, fast())
	if calls != 1 || err != wrapped || !repeat.IsPermanent(err) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestExecRetryIf(t *testing.T) {
	calls := 0
	err := repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return io.EOF
	}, fast(), repeat.WithRetryIf(func(err error) bool { return !errors.Is(err, io.EOF) }))
	if calls != 1 || err != io.EOF {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestExecRetryPolicyGetsAttempt(t *testing.T) {
	calls := 0
	_ = repeat.Exec(context.Background(), func(context.Context, int) error {
		calls++
		return errTemp
	}, fast(), repeat.WithMaxAttempts(10), repeat.WithRetryPolicy(func(attempt int, _ error) bool {
		return attempt < 1
	}))
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestExecOnRetryHook(t *testing.T) {
	var events []repeat.RetryEvent
	_ = repeat.Exec(context.Background(), func(context.Context, int) error {
		return errTemp
	}, fast(), repeat.WithMaxAttempts(3), repeat.WithOnRetry(func(e repeat.RetryEvent) {
		events = append(events, e)
	}))
	if len(events) != 2 || events[0].Attempt != 0 || events[1].Attempt != 1 || events[0].Delay != time.Millisecond {
		t.Fatalf("events: %+v", events)
	}
}

func TestExecCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := repeat.Exec(ctx, func(context.Context, int) error {
		calls++
		return nil
	})
	if calls != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestExecCancelDuringWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		go func() {
			time.Sleep(150 * time.Millisecond)
			cancel()
		}()
		err := repeat.Exec(ctx, func(context.Context, int) error {
			calls++
			return errTemp
		}, repeat.WithConstantDelay(time.Second), repeat.WithMaxAttempts(repeat.Unlimited))
		if calls != 1 {
			t.Fatalf("calls=%d", calls)
		}
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errTemp) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestExecNoAttemptAfterCancelInsideOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := repeat.Exec(ctx, func(context.Context, int) error {
		calls++
		cancel()
		return errTemp
	}, fast())
	if calls != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestExecGivesUpBeforeDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := repeat.Exec(ctx, func(context.Context, int) error { return errTemp },
			repeat.WithConstantDelay(time.Second))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v", err)
		}
		if time.Since(start) != 0 {
			t.Fatalf("should not wait for a delay that crosses the deadline")
		}
	})
}

func TestExecMaxElapsed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := repeat.Exec(context.Background(), func(context.Context, int) error {
			calls++
			return errTemp
		}, repeat.WithConstantDelay(time.Second), repeat.WithMaxAttempts(repeat.Unlimited),
			repeat.WithMaxElapsed(3500*time.Millisecond))
		if calls != 4 || !errors.Is(err, repeat.ErrMaxElapsed) {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}

func TestExecRetryAfterHint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		err := repeat.Exec(context.Background(), func(_ context.Context, attempt int) error {
			if attempt == 0 {
				return repeat.RetryAfter(errTemp, 5*time.Second)
			}
			return nil
		}, repeat.WithConstantDelay(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		if got := time.Since(start); got != 5*time.Second {
			t.Fatalf("waited %v", got)
		}
	})
}

func TestExecBackoffSequence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var delays []time.Duration
		_ = repeat.Exec(context.Background(), func(context.Context, int) error { return errTemp },
			repeat.WithBackoff(100*time.Millisecond, 500*time.Millisecond),
			repeat.WithJitter(repeat.JitterNone),
			repeat.WithMaxAttempts(6),
			repeat.WithOnRetry(func(e repeat.RetryEvent) { delays = append(delays, e.Delay) }))
		want := []time.Duration{100, 200, 400, 500, 500}
		for i := range want {
			if delays[i] != want[i]*time.Millisecond {
				t.Fatalf("delays=%v", delays)
			}
		}
	})
}

func TestJitterBounds(t *testing.T) {
	for _, j := range []repeat.Jitter{repeat.JitterFull, repeat.JitterEqual, repeat.JitterDecorrelated} {
		synctest.Test(t, func(t *testing.T) {
			base, limit := 10*time.Millisecond, 80*time.Millisecond
			_ = repeat.Exec(context.Background(), func(context.Context, int) error { return errTemp },
				repeat.WithBackoff(base, limit), repeat.WithJitter(j), repeat.WithMaxAttempts(50),
				repeat.WithOnRetry(func(e repeat.RetryEvent) {
					uncapped := repeat.Backoff(base, limit, e.Attempt)
					lo, hi := time.Duration(0), uncapped
					switch j {
					case repeat.JitterEqual:
						lo = uncapped - uncapped/2
					case repeat.JitterDecorrelated:
						lo, hi = base, limit
					}
					if e.Delay < lo || e.Delay > hi {
						t.Errorf("jitter %d attempt %d: delay %v not in [%v, %v]", j, e.Attempt, e.Delay, lo, hi)
					}
				}))
		})
	}
}

func TestBackoffOverflow(t *testing.T) {
	limit := time.Hour
	for _, a := range []int{0, 10, 40, 62, 63, 64, 1000} {
		d := repeat.Backoff(time.Second, limit, a)
		if d <= 0 || d > limit {
			t.Fatalf("attempt %d: %v", a, d)
		}
	}
	if d := repeat.Backoff(time.Nanosecond, 1<<62, 61); d != 1<<61 {
		t.Fatalf("got %v", d)
	}
}

func TestInvalidConfig(t *testing.T) {
	cases := [][]repeat.Option{
		{repeat.WithMaxAttempts(0)},
		{repeat.WithMaxAttempts(-2)},
		{repeat.WithBackoff(2*time.Second, time.Second)},
		{repeat.WithBaseDelay(-1)},
		{repeat.WithMaxElapsed(-1)},
		{repeat.WithJitter(repeat.Jitter(42))},
	}
	for i, opts := range cases {
		calls := 0
		err := repeat.Exec(context.Background(), func(context.Context, int) error { calls++; return nil }, opts...)
		if !errors.Is(err, repeat.ErrInvalidConfig) || calls != 0 {
			t.Fatalf("case %d: err=%v calls=%d", i, err, calls)
		}
	}
}

func TestDo(t *testing.T) {
	v, err := repeat.Do(context.Background(), func(context.Context) (int, error) {
		return 42, nil
	})
	if err != nil || v != 42 {
		t.Fatalf("v=%d err=%v", v, err)
	}
	v, err = repeat.Do(context.Background(), func(context.Context) (int, error) {
		return 7, errTemp
	}, fast(), repeat.WithMaxAttempts(2))
	if v != 0 || !errors.Is(err, errTemp) {
		t.Fatalf("v=%d err=%v", v, err)
	}
	if _, err := repeat.Do(context.Background(), func(context.Context) (int, error) { return 0, nil },
		repeat.WithMaxAttempts(0)); !errors.Is(err, repeat.ErrInvalidConfig) {
		t.Fatalf("err=%v", err)
	}
}

func TestRetrierConcurrent(t *testing.T) {
	r, err := repeat.New(fast(), repeat.WithMaxAttempts(3))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	for range 16 {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = r.Exec(context.Background(), func(_ context.Context, a int) error {
				if a < 2 {
					return errTemp
				}
				return nil
			})
		}()
	}
	for range 16 {
		<-done
	}
}

func TestErrorMessage(t *testing.T) {
	e := &repeat.Error{Attempts: 1, Err: io.EOF}
	if e.Error() != "repeat: gave up after 1 attempt: EOF" {
		t.Fatal(e.Error())
	}
	e = &repeat.Error{Attempts: 3, Err: io.EOF, Cause: repeat.ErrMaxElapsed}
	if e.Error() != "repeat: gave up after 3 attempts (repeat: max elapsed time exceeded): EOF" {
		t.Fatal(e.Error())
	}
}

func BenchmarkRetrierExecSuccess(b *testing.B) {
	r, _ := repeat.New()
	op := func(context.Context, int) error { return nil }
	ctx := context.Background()
	for b.Loop() {
		_ = r.Exec(ctx, op)
	}
}

func BenchmarkExecWithOptions(b *testing.B) {
	op := func(context.Context, int) error { return nil }
	ctx := context.Background()
	for b.Loop() {
		_ = repeat.Exec(ctx, op, repeat.WithMaxAttempts(3))
	}
}

func BenchmarkRetrierExecRetries(b *testing.B) {
	r, _ := repeat.New(repeat.WithConstantDelay(0), repeat.WithMaxAttempts(3))
	op := func(_ context.Context, a int) error {
		if a < 2 {
			return errTemp
		}
		return nil
	}
	ctx := context.Background()
	for b.Loop() {
		_ = r.Exec(ctx, op)
	}
}
