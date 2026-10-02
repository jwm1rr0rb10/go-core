package tcp

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

const (
	defaultBufferSize   = 4096
	defaultDialTimeout  = 5 * time.Second
	defaultReadTimeout  = 4 * time.Second
	defaultWriteTimeout = 4 * time.Second
)

// aLongTimeAgo is a deadline in the past used to interrupt blocked I/O.
var aLongTimeAgo = time.Unix(1, 0)

// ConnectionStats is a snapshot of client counters.
type ConnectionStats struct {
	BytesRead    uint64
	BytesWritten uint64
	LastActivity time.Time
	// RetryCount is the total number of retries performed by Do,
	// WriteWithRetry and ReadWithRetry.
	RetryCount int
	// Reconnects is the number of successful reconnects.
	Reconnects uint64
}

// ClientOption configures a [Client].
type ClientOption func(*Client)

// WithTimeouts sets per-operation read and write timeouts (defaults 4s).
// 0 disables the timeout; the context deadline still applies.
func WithTimeouts(read, write time.Duration) ClientOption {
	return func(c *Client) {
		c.readTimeout = max(read, 0)
		c.writeTimeout = max(write, 0)
	}
}

// WithDialTimeout sets the connect timeout (default 5s).
func WithDialTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.dialer.Timeout = max(d, 0) }
}

// WithDialer replaces the net.Dialer (keep-alive, local address, control
// hooks). Its Timeout is used as the dial timeout.
func WithDialer(d net.Dialer) ClientOption {
	return func(c *Client) { c.dialer = d }
}

// WithBufferSize sets the read buffer size (default 4096). It is also the
// maximum chunk returned by Read.
func WithBufferSize(size int) ClientOption {
	return func(c *Client) {
		if size > 0 {
			c.bufferSize = size
		}
	}
}

// WithMaxFrameSize sets the limit for ReadFrame/WriteFrame
// (default DefaultMaxFrameSize).
func WithMaxFrameSize(n int) ClientOption {
	return func(c *Client) {
		if n > 0 {
			c.maxFrameSize = n
		}
	}
}

// WithClientLogger sets a logger for reconnect/retry diagnostics
// (default: discard).
func WithClientLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithTLSClientConfig enables TLS. If cfg.ServerName is empty it is derived
// from the address.
func WithTLSClientConfig(cfg *tls.Config) ClientOption {
	return func(c *Client) { c.tlsConfig = cfg }
}

// WithRetryPolicy sets the policy used by Do, WriteWithRetry and ReadWithRetry.
func WithRetryPolicy(p RetryPolicy) ClientOption {
	return func(c *Client) { c.retry = p.normalize() }
}

// Client is a TCP client. One goroutine may read while another writes
// (like net.Conn); Close, Reconnect and Stats are safe from any goroutine.
// After Close every method returns [ErrClientClosed]: a closed client is never
// revived by Reconnect or retries.
type Client struct {
	address      string
	dialer       net.Dialer
	tlsConfig    *tls.Config
	readTimeout  time.Duration
	writeTimeout time.Duration
	bufferSize   int
	maxFrameSize int
	retry        RetryPolicy
	logger       *slog.Logger

	mu          sync.Mutex
	conn        net.Conn
	br          *bufio.Reader
	closed      bool
	connectedAt time.Time

	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	lastActivity atomic.Int64
	retries      atomic.Int64
	reconnects   atomic.Uint64
}

// NewClient creates a client for address. It does not connect; call Connect.
func NewClient(address string, opts ...ClientOption) (*Client, error) {
	if address == "" {
		return nil, errors.New("tcp: address cannot be empty")
	}
	c := &Client{
		address:      address,
		dialer:       net.Dialer{Timeout: defaultDialTimeout},
		readTimeout:  defaultReadTimeout,
		writeTimeout: defaultWriteTimeout,
		bufferSize:   defaultBufferSize,
		maxFrameSize: DefaultMaxFrameSize,
		retry:        DefaultRetryPolicy,
		logger:       slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Dial creates a client and connects it.
func Dial(ctx context.Context, address string, opts ...ClientOption) (*Client, error) {
	c, err := NewClient(address, opts...)
	if err != nil {
		return nil, err
	}
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Connect dials the server. It is a no-op if already connected. Dialing
// honours ctx and the dial timeout and never holds the client lock.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	switch {
	case c.closed:
		c.mu.Unlock()
		return ErrClientClosed
	case c.conn != nil:
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	var (
		conn net.Conn
		err  error
	)
	if c.tlsConfig != nil {
		d := tls.Dialer{NetDialer: &c.dialer, Config: c.tlsConfig}
		conn, err = d.DialContext(ctx, network, c.address)
	} else {
		conn, err = c.dialer.DialContext(ctx, network, c.address)
	}
	if err != nil {
		return wrapError("dial", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = conn.Close()
		return ErrClientClosed
	}
	if c.conn != nil { // a concurrent Connect won
		_ = conn.Close()
		return nil
	}
	c.conn = conn
	c.br = bufio.NewReaderSize(conn, c.bufferSize)
	c.connectedAt = time.Now()
	c.lastActivity.Store(c.connectedAt.UnixNano())
	return nil
}

// Read returns the next chunk of data (at most the buffer size). The result
// is a fresh slice the caller owns; use ReadInto to avoid the allocation.
func (c *Client) Read(ctx context.Context) ([]byte, error) {
	buf := make([]byte, c.bufferSize)
	n, err := c.ReadInto(ctx, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// ReadInto reads into p and returns the number of bytes read. It does not
// allocate.
func (c *Client) ReadInto(ctx context.Context, p []byte) (int, error) {
	op, err := c.begin(ctx, opRead)
	if err != nil {
		return 0, err
	}
	n, err := op.br.Read(p)
	return n, c.end(ctx, op, n, err)
}

// ReadFull reads exactly len(p) bytes.
func (c *Client) ReadFull(ctx context.Context, p []byte) error {
	op, err := c.begin(ctx, opRead)
	if err != nil {
		return err
	}
	n, err := io.ReadFull(op.br, p)
	return c.end(ctx, op, n, err)
}

// Write writes all of data.
func (c *Client) Write(ctx context.Context, data []byte) error {
	op, err := c.begin(ctx, opWrite)
	if err != nil {
		return err
	}
	n, err := op.conn.Write(data)
	return c.end(ctx, op, n, err)
}

// WriteFrame writes payload as one length-prefixed frame (see [WriteFrame]).
func (c *Client) WriteFrame(ctx context.Context, payload []byte) error {
	op, err := c.begin(ctx, opWrite)
	if err != nil {
		return err
	}
	n := 0
	if err = WriteFrame(op.conn, payload, c.maxFrameSize); err == nil {
		n = len(payload) + FrameHeaderSize
	}
	return c.end(ctx, op, n, err)
}

// ReadFrame reads one length-prefixed frame, reusing buf when it is large
// enough (see [ReadFrame]).
func (c *Client) ReadFrame(ctx context.Context, buf []byte) ([]byte, error) {
	op, err := c.begin(ctx, opRead)
	if err != nil {
		return buf[:0], err
	}
	out, err := ReadFrame(op.br, buf, c.maxFrameSize)
	n := 0
	if err == nil {
		n = len(out) + FrameHeaderSize
	}
	if err = c.end(ctx, op, n, err); err != nil {
		return out[:0], err
	}
	return out, nil
}

type opKind uint8

const (
	opRead opKind = iota
	opWrite
)

func (k opKind) String() string {
	if k == opWrite {
		return "write"
	}
	return "read"
}

// ioOp is the per-operation state shared by begin and end. Keeping it a
// plain value (no closures, no method values) keeps the I/O path
// allocation-free unless ctx is cancellable.
type ioOp struct {
	kind     opKind
	conn     net.Conn
	br       *bufio.Reader
	ctxBound bool
	stop     func() bool
}

// begin snapshots the connection, applies the deadline derived from the
// operation timeout and ctx, and arms interruption on ctx cancellation.
func (c *Client) begin(ctx context.Context, kind opKind) (ioOp, error) {
	if err := ctx.Err(); err != nil {
		return ioOp{}, &ConnectionError{Op: kind.String(), Err: err}
	}
	c.mu.Lock()
	op := ioOp{kind: kind, conn: c.conn, br: c.br}
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ioOp{}, ErrClientClosed
	}
	if op.conn == nil {
		return ioOp{}, &ConnectionError{Op: kind.String(), Err: ErrConnectionClosed, IsRetryable: true}
	}

	timeout := c.readTimeout
	if kind == opWrite {
		timeout = c.writeTimeout
	}
	var dl time.Time
	if timeout > 0 {
		dl = time.Now().Add(timeout)
	}
	if ctxDL, ok := ctx.Deadline(); ok && (dl.IsZero() || ctxDL.Before(dl)) {
		dl, op.ctxBound = ctxDL, true
	}
	if err := setDeadline(op.conn, kind, dl); err != nil {
		return ioOp{}, c.mapErr(ctx, kind.String(), err, false)
	}
	if ctx.Done() != nil {
		conn := op.conn
		op.stop = context.AfterFunc(ctx, func() { _ = setDeadline(conn, kind, aLongTimeAgo) })
	}
	return op, nil
}

// end disarms cancellation, updates statistics and maps the error.
func (c *Client) end(ctx context.Context, op ioOp, n int, err error) error {
	fired := false
	if op.stop != nil {
		fired = !op.stop()
	}
	if n > 0 {
		if op.kind == opWrite {
			c.bytesWritten.Add(uint64(n))
		} else {
			c.bytesRead.Add(uint64(n))
		}
		c.lastActivity.Store(time.Now().UnixNano())
	}
	if err == nil {
		return nil
	}
	if fired && ctx.Err() != nil {
		return &ConnectionError{Op: op.kind.String(), Err: ctx.Err()}
	}
	return c.mapErr(ctx, op.kind.String(), err, op.ctxBound)
}

func setDeadline(conn net.Conn, kind opKind, t time.Time) error {
	if kind == opWrite {
		return conn.SetWriteDeadline(t)
	}
	return conn.SetReadDeadline(t)
}

// mapErr converts an I/O error. ctxBound reports that the deadline came from
// ctx, so a timeout means the context deadline passed even if ctx.Err() is
// not set yet (the two timers race).
func (c *Client) mapErr(ctx context.Context, op string, err error, ctxBound bool) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	switch {
	case closed:
		return ErrClientClosed
	case isTimeout(err):
		if ctxErr := ctx.Err(); ctxErr != nil {
			return &ConnectionError{Op: op, Err: ctxErr}
		}
		if ctxBound {
			return &ConnectionError{Op: op, Err: context.DeadlineExceeded}
		}
		return &ConnectionError{Op: op, Err: ErrTimeout, IsRetryable: true}
	case errors.Is(err, net.ErrClosed):
		return &ConnectionError{Op: op, Err: ErrConnectionClosed, IsRetryable: true}
	default:
		return wrapError(op, err)
	}
}

// Close closes the connection permanently. It is idempotent and safe to call
// concurrently with I/O, which is interrupted with [ErrClientClosed].
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn, c.br = nil, nil
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return wrapError("close", err)
	}
	return nil
}

// Reconnect drops the current connection (if any) and dials a new one.
// It returns [ErrClientClosed] if Close was called.
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientClosed
	}
	old := c.conn
	c.conn, c.br = nil, nil
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if err := c.Connect(ctx); err != nil {
		return err
	}
	c.reconnects.Add(1)
	c.logger.Debug("tcp: reconnected", "addr", c.address)
	return nil
}

// Connected reports whether the client currently holds a connection.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// RemoteAddr returns the remote address, or nil when not connected.
func (c *Client) RemoteAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local address, or nil when not connected.
func (c *Client) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.LocalAddr()
}

// Stats returns a snapshot of client counters.
func (c *Client) Stats() ConnectionStats {
	st := ConnectionStats{
		BytesRead:    c.bytesRead.Load(),
		BytesWritten: c.bytesWritten.Load(),
		RetryCount:   int(c.retries.Load()),
		Reconnects:   c.reconnects.Load(),
	}
	if la := c.lastActivity.Load(); la > 0 {
		st.LastActivity = time.Unix(0, la)
	}
	return st
}

// state returns the raw connection, read buffer and connect time for the
// pool's health checks.
func (c *Client) state() (net.Conn, *bufio.Reader, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn, c.br, c.connectedAt
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
