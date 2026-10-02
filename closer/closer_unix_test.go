//go:build unix

package closer_test

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/closer"
)

func TestCloseOnSignal(t *testing.T) {
	lc := closer.NewLIFOCloser()
	var closed atomic.Bool
	lc.AddNoErr(closer.NoErrCloserFunc(func() { closed.Store(true) }))
	done := make(chan error, 1)
	go func() { done <- closer.CloseOnSignal(lc, syscall.SIGUSR1) }()
	deadline := time.After(5 * time.Second)
	for {
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
		select {
		case err := <-done:
			if err != nil || !closed.Load() {
				t.Fatalf("err=%v closed=%v", err, closed.Load())
			}
			return
		case <-deadline:
			t.Fatal("signal not handled")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
