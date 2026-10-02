package tcp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestServerMaxConnections(t *testing.T) {
	hold := make(chan struct{})
	s := startServer(t, func(_ context.Context, conn net.Conn) { <-hold }, WithMaxConnections(1))
	defer close(hold)

	dialClient(t, s)
	waitFor(t, time.Second, func() bool { return s.Stats().ActiveConnections == 1 })

	c2 := dialClient(t, s)
	if _, err := c2.Read(ctxTimeout(t, time.Second)); err == nil {
		t.Fatal("second connection should be rejected")
	}
	waitFor(t, time.Second, func() bool { return s.Stats().RejectedConnections == 1 })
}

func TestServerSetMaxConnectionsConcurrent(t *testing.T) {
	s := startServer(t, echoHandler)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range 100 {
				s.SetMaxConnections(int64(i*100 + j))
			}
		})
		wg.Go(func() {
			for range 10 {
				c, err := Dial(context.Background(), s.Addr().String())
				if err == nil {
					_ = c.Close()
				}
				_ = s.Stats()
			}
		})
	}
	wg.Wait()
	s.SetMaxConnections(-5)
	if got := s.maxConns.Load(); got != 0 {
		t.Fatalf("negative limit should mean unlimited, got %d", got)
	}
}
