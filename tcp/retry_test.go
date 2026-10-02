package tcp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryPolicyBackoff(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: 80 * time.Millisecond}.normalize()
	for attempt := range 100 {
		ceiling := min(p.MaxDelay, p.BaseDelay<<min(attempt, 30))
		for range 20 {
			if d := p.Backoff(attempt); d < 0 || d > ceiling {
				t.Fatalf("attempt %d: %v outside [0,%v]", attempt, d, ceiling)
			}
		}
	}
	if d := (RetryPolicy{}).Backoff(3); d != 0 {
		t.Fatalf("zero policy should not sleep, got %v", d)
	}
}

func TestRetryPolicyNormalize(t *testing.T) {
	p := RetryPolicy{MaxAttempts: -1, BaseDelay: time.Second, MaxDelay: time.Millisecond}.normalize()
	if p.MaxAttempts != 1 || p.MaxDelay != time.Second {
		t.Fatalf("normalize: %+v", p)
	}
}

func TestSleepCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
