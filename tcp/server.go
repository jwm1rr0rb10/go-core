package tcp

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

const (
	defaultIdleTimeout = 5 * time.Minute
	minAcceptBackoff   = 5 * time.Millisecond
	maxAcceptBackoff   = time.Second
)

// HandlerFunc serves one accepted connection. ctx is cancelled when the
// server starts shutting down; long-lived handlers should watch it and
// return. The server closes conn after the handler returns, so handlers do
// not need to.
type HandlerFunc func(ctx context.Context, conn net.Conn)

// Middleware runs before the handler. It may wrap the connection (for example
// to keep data it buffered) and returns the connection the next stage should
// use. A non-nil error rejects the connection: the server closes it and counts
// it in [ServerStats.RejectedConnections].
type Middleware func(ctx context.Context, conn net.Conn) (net.Conn, error)

// ServerStats is a point-in-time snapshot of server counters.
type ServerStats struct {
	ActiveConnections   int64     // connections currently being served
	TotalConnections    int64     // connections accepted since start
	RejectedConnections int64     // closed by the connection limit or middleware
	BytesRead           int64     // bytes read from all connections, live and closed
	BytesWritten        int64     // bytes written to all connections, live and closed
	LastActivity        time.Time // last accept, read or write
}

// ServerOption configures a [Server].
type ServerOption func(*Server)

// WithServerLogger sets the logger for accept errors, rejections and handler
// panics. The default discards everything.
func WithServerLogger(l *slog.Logger) ServerOption {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithServerTLS makes the server terminate TLS on every listener it serves.
func WithServerTLS(cfg *tls.Config) ServerOption {
	return func(s *Server) { s.tlsConfig = cfg }
}

// WithIdleTimeout sets the sliding idle timeout (default 5 minutes): a
// connection is closed when no read (or no write) completes for d. Use 0 to
// disable and manage deadlines in the handler. Handlers may still set their
// own deadlines; doing so suspends automatic refreshing for that direction
// until the deadline is cleared with a zero time.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		if d >= 0 {
			s.idleTimeout = d
		}
	}
}

// WithServerTimeout is an alias for [WithIdleTimeout].
//
// Deprecated: use WithIdleTimeout.
func WithServerTimeout(d time.Duration) ServerOption { return WithIdleTimeout(d) }

// WithMaxConnections limits concurrently served connections. Extra
// connections are accepted and closed immediately. 0 means unlimited.
func WithMaxConnections(n int64) ServerOption {
	return func(s *Server) { s.maxConns.Store(max(n, 0)) }
}

// WithMiddleware appends middleware; they run in the given order.
func WithMiddleware(mw ...Middleware) ServerOption {
	return func(s *Server) {
		for _, m := range mw {
			if m != nil {
				s.middleware = append(s.middleware, m)
			}
		}
	}
}

// WithListenConfig sets the net.ListenConfig used by ListenAndServe and Start
// (for SO_REUSEPORT, keep-alive tuning, etc.).
func WithListenConfig(lc net.ListenConfig) ServerOption {
	return func(s *Server) { s.listenConfig = lc }
}

// WithBaseContext sets the parent of the context passed to handlers.
func WithBaseContext(ctx context.Context) ServerOption {
	return func(s *Server) {
		if ctx != nil {
			s.baseCtx = ctx
		}
	}
}

// Server is a TCP server. Create it with [NewServer]; it is safe for
// concurrent use. A Server cannot be restarted after Shutdown or Close.
type Server struct {
	address      string
	handler      HandlerFunc
	logger       *slog.Logger
	tlsConfig    *tls.Config
	idleTimeout  time.Duration
	middleware   []Middleware
	listenConfig net.ListenConfig
	baseCtx      context.Context

	maxConns atomic.Int64
	active   atomic.Int64
	total    atomic.Int64
	rejected atomic.Int64
	// Totals of connections that already finished; live ones are summed on demand.
	closedRead    atomic.Int64
	closedWritten atomic.Int64
	lastAccept    atomic.Int64
	lastClosed    atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	listeners map[net.Listener]struct{}
	conns     map[*serverConn]struct{}
	shutdown  atomic.Bool
	wg        sync.WaitGroup
}

// NewServer creates a server that will listen on address (used by
// ListenAndServe and Start; Serve accepts any listener).
func NewServer(address string, handler HandlerFunc, opts ...ServerOption) (*Server, error) {
	if handler == nil {
		return nil, errors.New("tcp: handler cannot be nil")
	}
	s := &Server{
		address:     address,
		handler:     handler,
		logger:      slog.New(slog.DiscardHandler),
		idleTimeout: defaultIdleTimeout,
		baseCtx:     context.Background(),
		listeners:   make(map[net.Listener]struct{}),
		conns:       make(map[*serverConn]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.ctx, s.cancel = context.WithCancel(s.baseCtx)
	return s, nil
}

// SetMaxConnections changes the concurrent connection limit at runtime
// (0 = unlimited). It is safe to call while serving.
func (s *Server) SetMaxConnections(n int64) { s.maxConns.Store(max(n, 0)) }

// ListenAndServe listens on the configured address and serves until Shutdown
// or Close, returning [ErrServerClosed] in that case.
func (s *Server) ListenAndServe() error {
	ln, err := s.listen()
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Start binds the listener synchronously (so bind errors are returned) and
// serves in a background goroutine. Use Addr to learn the bound address.
func (s *Server) Start() error {
	ln, err := s.listen()
	if err != nil {
		return err
	}
	if !s.track(ln) {
		_ = ln.Close()
		return ErrServerClosed
	}
	go func() {
		if err := s.serve(ln); err != nil && !errors.Is(err, ErrServerClosed) {
			s.logger.Error("tcp: serve stopped", "addr", ln.Addr().String(), "err", err)
		}
	}()
	return nil
}

func (s *Server) listen() (net.Listener, error) {
	if s.address == "" {
		return nil, errors.New("tcp: address cannot be empty")
	}
	if s.shutdown.Load() {
		return nil, ErrServerClosed
	}
	ln, err := s.listenConfig.Listen(s.ctx, network, s.address)
	if err != nil {
		return nil, wrapError("listen", err)
	}
	return ln, nil
}

// Serve accepts connections on ln until Shutdown or Close and always closes
// ln before returning. If TLS is configured, ln is wrapped with tls.NewListener.
func (s *Server) Serve(ln net.Listener) error {
	if !s.track(ln) {
		_ = ln.Close()
		return ErrServerClosed
	}
	return s.serve(ln)
}

func (s *Server) track(ln net.Listener) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown.Load() {
		return false
	}
	s.listeners[ln] = struct{}{}
	return true
}

func (s *Server) serve(raw net.Listener) error {
	defer func() {
		s.mu.Lock()
		delete(s.listeners, raw)
		s.mu.Unlock()
		_ = raw.Close()
	}()
	ln := raw
	if s.tlsConfig != nil {
		ln = tls.NewListener(raw, s.tlsConfig)
	}

	var backoff time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.shutdown.Load() {
				return ErrServerClosed
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			// Everything else (EMFILE, ENFILE, ECONNABORTED, ...) is treated as
			// transient: back off instead of spinning on the CPU.
			backoff = min(max(backoff*2, minAcceptBackoff), maxAcceptBackoff)
			s.logger.Warn("tcp: accept error; retrying", "err", err, "backoff", backoff)
			t := time.NewTimer(backoff)
			select {
			case <-t.C:
			case <-s.ctx.Done():
				t.Stop()
				return ErrServerClosed
			}
			continue
		}
		backoff = 0
		now := time.Now()
		s.lastAccept.Store(now.UnixNano())

		sc := newServerConn(conn, s.idleTimeout, now)
		switch s.addConn(sc) {
		case connAdded:
			go s.handle(sc)
		case connOverLimit:
			s.rejected.Add(1)
			_ = conn.Close()
		case connShutdown:
			_ = conn.Close()
			return ErrServerClosed
		}
	}
}

type addResult uint8

const (
	connAdded addResult = iota
	connOverLimit
	connShutdown
)

// addConn registers a connection. The limit check happens under the same lock
// as the increment, so the limit is exact even with several listeners.
func (s *Server) addConn(sc *serverConn) addResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown.Load() {
		return connShutdown
	}
	if limit := s.maxConns.Load(); limit > 0 && s.active.Load() >= limit {
		return connOverLimit
	}
	s.conns[sc] = struct{}{}
	s.active.Add(1)
	s.total.Add(1)
	s.wg.Add(1)
	return connAdded
}

func (s *Server) removeConn(sc *serverConn) {
	s.mu.Lock()
	delete(s.conns, sc)
	s.closedRead.Add(sc.bytesRead.Load())
	s.closedWritten.Add(sc.bytesWritten.Load())
	if la := sc.lastActivity.Load(); la > s.lastClosed.Load() {
		s.lastClosed.Store(la)
	}
	s.mu.Unlock()
	s.active.Add(-1)
	s.wg.Done()
}

func (s *Server) handle(sc *serverConn) {
	defer s.removeConn(sc)
	defer func() { _ = sc.Close() }()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("tcp: handler panic", "remote", sc.RemoteAddr().String(),
				"panic", r, "stack", string(debug.Stack()))
		}
	}()

	var conn net.Conn = sc
	for _, mw := range s.middleware {
		next, err := mw(s.ctx, conn)
		if err != nil {
			s.rejected.Add(1)
			s.logger.Debug("tcp: connection rejected", "remote", sc.RemoteAddr().String(), "err", err)
			return
		}
		if next != nil {
			conn = next
		}
	}
	s.handler(s.ctx, conn)
}

// Addr returns the address of one of the active listeners, or nil.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ln := range s.listeners {
		return ln.Addr()
	}
	return nil
}

// Shutdown gracefully stops the server: it closes all listeners, cancels the
// handler context and waits for handlers to return. If ctx expires first, all
// remaining connections are force-closed and ctx.Err() is returned. Shutdown
// never holds a lock while waiting, so Stats keeps working.
func (s *Server) Shutdown(ctx context.Context) error {
	s.beginShutdown()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.closeConns()
		return ctx.Err()
	}
}

// Close stops the server immediately: listeners and all connections are
// closed, and Close waits for handlers to return.
func (s *Server) Close() error {
	s.beginShutdown()
	s.closeConns()
	s.wg.Wait()
	return nil
}

// Stop is Shutdown without a deadline.
//
// Deprecated: use Shutdown or Close.
func (s *Server) Stop() error { return s.Shutdown(context.Background()) }

// StopWithTimeout is Shutdown with a timeout. On timeout it force-closes the
// remaining connections and returns [ErrTimeout]; it never leaks goroutines.
//
// Deprecated: use Shutdown with context.WithTimeout.
func (s *Server) StopWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		return ErrTimeout
	}
	return nil
}

func (s *Server) beginShutdown() {
	s.mu.Lock()
	s.shutdown.Store(true)
	lns := make([]net.Listener, 0, len(s.listeners))
	for ln := range s.listeners {
		lns = append(lns, ln)
	}
	s.mu.Unlock()
	s.cancel()
	for _, ln := range lns {
		_ = ln.Close()
	}
}

func (s *Server) closeConns() {
	s.mu.Lock()
	conns := make([]*serverConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Conn.Close()
	}
}

// Stats returns a snapshot of the server counters. It is cheap enough to call
// from a metrics scrape (it walks live connections under a short lock).
func (s *Server) Stats() ServerStats {
	st := ServerStats{
		ActiveConnections:   s.active.Load(),
		TotalConnections:    s.total.Load(),
		RejectedConnections: s.rejected.Load(),
	}
	last := max(s.lastAccept.Load(), s.lastClosed.Load())

	s.mu.Lock()
	st.BytesRead = s.closedRead.Load()
	st.BytesWritten = s.closedWritten.Load()
	for c := range s.conns {
		st.BytesRead += c.bytesRead.Load()
		st.BytesWritten += c.bytesWritten.Load()
		last = max(last, c.lastActivity.Load())
	}
	s.mu.Unlock()

	if last > 0 {
		st.LastActivity = time.Unix(0, last)
	}
	return st
}
