package tcp

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

const defaultMaxIdle = 8

// Factory creates a connected client for a [Pool]. It should honour ctx.
// If the returned client is not connected, the pool connects it.
type Factory func(ctx context.Context) (*Client, error)

// PoolOption configures a [Pool].
type PoolOption func(*Pool)

// WithMaxIdle sets how many idle clients are kept (default 8). Extra clients
// returned with Put are closed.
func WithMaxIdle(n int) PoolOption {
	return func(p *Pool) { p.maxIdle = max(n, 0) }
}

// WithMaxActive limits clients handed out at the same time (idle ones are not
// counted). Get blocks until a slot frees up or ctx is done. 0 = unlimited.
func WithMaxActive(n int) PoolOption {
	return func(p *Pool) {
		if n > 0 {
			p.sem = make(chan struct{}, n)
		} else {
			p.sem = nil
		}
	}
}

// WithPoolIdleTimeout closes clients that stayed idle in the pool longer
// than d (checked lazily on Get and by Prune). 0 = no limit.
func WithPoolIdleTimeout(d time.Duration) PoolOption {
	return func(p *Pool) { p.idleTimeout = max(d, 0) }
}

// WithPoolMaxLifetime closes clients whose connection is older than d, which
// helps rebalance load behind L4 balancers. 0 = no limit.
func WithPoolMaxLifetime(d time.Duration) PoolOption {
	return func(p *Pool) { p.maxLifetime = max(d, 0) }
}

// WithPoolHealthCheck enables or disables the liveness check on Get
// (default enabled). See [Pool.Get].
func WithPoolHealthCheck(enabled bool) PoolOption {
	return func(p *Pool) { p.healthCheck = enabled }
}

// WithPoolLogger sets a logger for evictions (default: discard).
func WithPoolLogger(l *slog.Logger) PoolOption {
	return func(p *Pool) {
		if l != nil {
			p.logger = l
		}
	}
}

// PoolStats is a snapshot of pool counters.
type PoolStats struct {
	Idle      int    // clients waiting in the pool
	InUse     int    // clients handed out and not yet returned
	Hits      uint64 // Get served from an idle client
	Misses    uint64 // Get had to create a client
	Evictions uint64 // idle clients closed as expired or dead
}

type idleClient struct {
	c     *Client
	since time.Time
}

// Pool is a LIFO pool of [Client]s. Every client obtained with Get must be
// given back with Put (healthy) or Discard (broken); otherwise its
// max-active slot is never released.
type Pool struct {
	factory     Factory
	maxIdle     int
	idleTimeout time.Duration
	maxLifetime time.Duration
	healthCheck bool
	logger      *slog.Logger
	sem         chan struct{}

	mu     sync.Mutex
	idle   []idleClient
	inUse  map[*Client]struct{}
	closed bool

	hits, misses, evictions atomic.Uint64
}

// NewPool creates a pool that uses factory to create clients.
func NewPool(factory Factory, opts ...PoolOption) (*Pool, error) {
	if factory == nil {
		return nil, errors.New("tcp: pool factory cannot be nil")
	}
	p := &Pool{
		factory:     factory,
		maxIdle:     defaultMaxIdle,
		healthCheck: true,
		logger:      slog.New(slog.DiscardHandler),
		inUse:       make(map[*Client]struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// Get returns an idle client or creates one. Idle clients are validated
// first: expired ones (idle timeout, max lifetime) are closed, and with the
// health check enabled the socket is peeked without blocking (MSG_PEEK on
// Unix) so connections closed or reset by the peer, or carrying unexpected
// unread data, are never handed out.
//
// With WithMaxActive, Get waits for a free slot until ctx is done.
func (p *Pool) Get(ctx context.Context) (*Client, error) {
	if err := p.acquire(ctx); err != nil {
		return nil, err
	}
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			p.release()
			return nil, ErrPoolClosed
		}
		n := len(p.idle)
		if n == 0 {
			p.mu.Unlock()
			break
		}
		ic := p.idle[n-1]
		p.idle[n-1] = idleClient{}
		p.idle = p.idle[:n-1]
		p.mu.Unlock()

		if p.usable(ic, time.Now()) {
			if p.markInUse(ic.c) {
				p.hits.Add(1)
				return ic.c, nil
			}
			_ = ic.c.Close()
			p.release()
			return nil, ErrPoolClosed
		}
		p.evict(ic.c, "stale")
	}

	p.misses.Add(1)
	c, err := p.factory(ctx)
	if err == nil && c == nil {
		err = errors.New("tcp: pool factory returned nil client")
	}
	if err == nil && !c.Connected() {
		err = c.Connect(ctx)
	}
	if err != nil {
		if c != nil {
			_ = c.Close()
		}
		p.release()
		return nil, err
	}
	if !p.markInUse(c) {
		_ = c.Close()
		p.release()
		return nil, ErrPoolClosed
	}
	return c, nil
}

// Put returns a healthy client to the pool. Clients that are closed or
// disconnected, clients beyond the idle limit and clients returned after the
// pool was closed are closed instead. Clients that did not come from this
// pool are ignored.
func (p *Pool) Put(c *Client) {
	if c == nil {
		return
	}
	p.mu.Lock()
	if _, ok := p.inUse[c]; !ok {
		p.mu.Unlock()
		return
	}
	delete(p.inUse, c)
	keep := !p.closed && len(p.idle) < p.maxIdle && !c.isClosed() && c.Connected()
	if keep {
		p.idle = append(p.idle, idleClient{c: c, since: time.Now()})
	}
	p.mu.Unlock()
	p.release()
	if !keep {
		_ = c.Close()
	}
}

// Discard closes a client obtained from Get (for example after an I/O error)
// and frees its slot.
func (p *Pool) Discard(c *Client) {
	if c == nil {
		return
	}
	p.mu.Lock()
	_, ok := p.inUse[c]
	delete(p.inUse, c)
	p.mu.Unlock()
	_ = c.Close()
	if ok {
		p.release()
	}
}

// Prune closes idle clients that have expired. Call it periodically if you
// configured an idle timeout or max lifetime and traffic is bursty.
func (p *Pool) Prune() int {
	now := time.Now()
	p.mu.Lock()
	var stale []*Client
	kept := p.idle[:0]
	for _, ic := range p.idle {
		if p.expired(ic, now) {
			stale = append(stale, ic.c)
		} else {
			kept = append(kept, ic)
		}
	}
	clear(p.idle[len(kept):])
	p.idle = kept
	p.mu.Unlock()
	for _, c := range stale {
		p.evict(c, "expired")
	}
	return len(stale)
}

// Close closes all idle clients and makes Get fail with [ErrPoolClosed].
// Clients in use are closed when they are returned. Close is idempotent.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()

	var errs error
	for _, ic := range idle {
		errs = errors.Append(errs, ic.c.Close())
	}
	return errs
}

// Stats returns a snapshot of pool counters.
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	idle, inUse := len(p.idle), len(p.inUse)
	p.mu.Unlock()
	return PoolStats{
		Idle:      idle,
		InUse:     inUse,
		Hits:      p.hits.Load(),
		Misses:    p.misses.Load(),
		Evictions: p.evictions.Load(),
	}
}

func (p *Pool) acquire(ctx context.Context) error {
	if p.sem == nil {
		return ctx.Err()
	}
	select {
	case p.sem <- struct{}{}:
		return nil
	default:
	}
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) release() {
	if p.sem != nil {
		<-p.sem
	}
}

func (p *Pool) markInUse(c *Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.inUse[c] = struct{}{}
	return true
}

func (p *Pool) expired(ic idleClient, now time.Time) bool {
	if p.idleTimeout > 0 && now.Sub(ic.since) > p.idleTimeout {
		return true
	}
	if p.maxLifetime > 0 {
		if _, _, at := ic.c.state(); now.Sub(at) > p.maxLifetime {
			return true
		}
	}
	return false
}

func (p *Pool) usable(ic idleClient, now time.Time) bool {
	if p.expired(ic, now) {
		return false
	}
	conn, br, _ := ic.c.state()
	if conn == nil {
		return false
	}
	if !p.healthCheck {
		return true
	}
	if br != nil && br.Buffered() > 0 {
		return false // leftover data from a previous exchange: out of sync
	}
	return connAlive(conn)
}

func (p *Pool) evict(c *Client, reason string) {
	p.evictions.Add(1)
	p.logger.Debug("tcp: evicting pooled client", "reason", reason)
	_ = c.Close()
}
