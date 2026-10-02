package tcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func newTestPool(t *testing.T, s *Server, opts ...PoolOption) *Pool {
	t.Helper()
	p, err := NewPool(func(ctx context.Context) (*Client, error) {
		return NewClient(s.Addr().String())
	}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestNewPoolValidation(t *testing.T) {
	if _, err := NewPool(nil); err == nil {
		t.Fatal("expected error for nil factory")
	}
}

func TestPoolReuse(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s)
	ctx := ctxTimeout(t, 2*time.Second)

	c1, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !c1.Connected() {
		t.Fatal("pool must connect clients the factory left unconnected")
	}
	p.Put(c1)
	c2, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Fatal("expected the idle client to be reused")
	}
	st := p.Stats()
	if st.Hits != 1 || st.Misses != 1 || st.InUse != 1 || st.Idle != 0 {
		t.Fatalf("stats: %+v", st)
	}
	p.Put(c2)
	p.Put(c2) // double Put is ignored
	if st := p.Stats(); st.Idle != 1 || st.InUse != 0 {
		t.Fatalf("after double put: %+v", st)
	}
}

func TestPoolDetectsDeadConnections(t *testing.T) {
	var mu sync.Mutex
	var serverConns []net.Conn
	s := startServer(t, func(ctx context.Context, conn net.Conn) {
		mu.Lock()
		serverConns = append(serverConns, conn)
		mu.Unlock()
		<-ctx.Done()
	})
	p := newTestPool(t, s)
	ctx := ctxTimeout(t, 2*time.Second)

	c1, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p.Put(c1)
	waitFor(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return len(serverConns) == 1 })
	mu.Lock()
	_ = serverConns[0].Close() // peer closes the idle pooled connection
	mu.Unlock()
	time.Sleep(20 * time.Millisecond)

	c2, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c2 == c1 {
		t.Fatal("a connection closed by the peer was handed out")
	}
	if p.Stats().Evictions != 1 {
		t.Fatalf("expected one eviction: %+v", p.Stats())
	}
	p.Put(c2)
}

func TestPoolMaxActive(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithMaxActive(1))
	c1, err := p.Get(ctxTimeout(t, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := p.Get(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected to wait for a slot, got %v", err)
	}

	got := make(chan *Client, 1)
	go func() {
		c, err := p.Get(ctxTimeout(t, 2*time.Second))
		if err != nil {
			t.Error(err)
		}
		got <- c
	}()
	time.Sleep(10 * time.Millisecond)
	p.Discard(c1) // frees the slot
	c2 := <-got
	if c2 == nil || c2 == c1 {
		t.Fatal("expected a new client after Discard")
	}
	p.Put(c2)
}

func TestPoolCloseIsSafe(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithMaxIdle(1))
	ctx := ctxTimeout(t, time.Second)
	a, _ := p.Get(ctx)
	b, _ := p.Get(ctx)
	p.Put(a)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	p.Put(b) // must not panic and must close b
	if !b.isClosed() || !a.isClosed() {
		t.Fatal("clients should be closed")
	}
	if _, err := p.Get(ctx); !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("Get after Close: %v", err)
	}
}

func TestPoolMaxIdle(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithMaxIdle(1))
	ctx := ctxTimeout(t, time.Second)
	a, _ := p.Get(ctx)
	b, _ := p.Get(ctx)
	p.Put(a)
	p.Put(b)
	if !b.isClosed() || p.Stats().Idle != 1 {
		t.Fatalf("extra idle client should be closed: %+v", p.Stats())
	}
}

func TestPoolIdleTimeoutAndPrune(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithPoolIdleTimeout(20*time.Millisecond))
	ctx := ctxTimeout(t, time.Second)
	c, _ := p.Get(ctx)
	p.Put(c)
	time.Sleep(40 * time.Millisecond)
	if n := p.Prune(); n != 1 || p.Stats().Idle != 0 || !c.isClosed() {
		t.Fatalf("prune removed %d, stats %+v", n, p.Stats())
	}
}

func TestPoolMaxLifetime(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithPoolMaxLifetime(20*time.Millisecond), WithPoolHealthCheck(false))
	ctx := ctxTimeout(t, time.Second)
	c1, _ := p.Get(ctx)
	p.Put(c1)
	time.Sleep(40 * time.Millisecond)
	c2, err := p.Get(ctx)
	if err != nil || c2 == c1 {
		t.Fatalf("expired client reused: %v", err)
	}
	p.Put(c2)
}

func TestPoolFactoryErrorReleasesSlot(t *testing.T) {
	boom := errors.New("boom")
	p, _ := NewPool(func(context.Context) (*Client, error) { return nil, boom }, WithMaxActive(1))
	for range 3 {
		if _, err := p.Get(context.Background()); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	}
	p2, _ := NewPool(func(context.Context) (*Client, error) { return nil, nil })
	if _, err := p2.Get(context.Background()); err == nil {
		t.Fatal("nil client must be an error")
	}
}

func TestPoolConcurrent(t *testing.T) {
	s := startServer(t, echoHandler)
	p := newTestPool(t, s, WithMaxActive(4), WithMaxIdle(4))
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			ctx := ctxTimeout(t, 5*time.Second)
			for range 20 {
				c, err := p.Get(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				if err := c.WriteFrame(ctx, []byte("x")); err != nil {
					p.Discard(c)
					continue
				}
				if _, err := c.ReadFrame(ctx, nil); err != nil {
					p.Discard(c)
					continue
				}
				p.Put(c)
			}
		})
	}
	wg.Wait()
	if st := p.Stats(); st.InUse != 0 || st.Idle > 4 {
		t.Fatalf("stats: %+v", st)
	}
}
