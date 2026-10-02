package tcp

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"math/bits"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

const (
	rateLimiterShards = 64
	shardBits         = 6
)

// Decision is the outcome of [RateLimiter.Check].
type Decision uint8

const (
	// Allow means the peer is within its rate.
	Allow Decision = iota
	// RequirePoW means the peer exceeded its soft rate and must solve a
	// proof-of-work challenge of the returned difficulty.
	RequirePoW
	// Deny means the peer is banned.
	Deny
)

// String implements fmt.Stringer.
func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case RequirePoW:
		return "require-pow"
	case Deny:
		return "deny"
	}
	return "unknown"
}

type rateLimiterConfig struct {
	softRate, hardRate   float64
	softBurst, hardBurst float64
	banDuration          time.Duration
	pow                  bool
	powHandshake         bool
	powTimeout           time.Duration
	minBits, maxBits     int
	decay                time.Duration
	v6PrefixLen          int
	maxPeers             int
	cleanupInterval      time.Duration
	now                  func() time.Time
	logger               *slog.Logger
}

// RateLimiterOption configures a [RateLimiter].
type RateLimiterOption func(*rateLimiterConfig)

// WithConnectionRate sets the soft limit (default 10/s, burst 10). Peers
// above it must solve proof of work (or are rejected when PoW is disabled).
func WithConnectionRate(perSecond float64, burst int) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if perSecond > 0 && burst > 0 {
			c.softRate, c.softBurst = perSecond, float64(burst)
		}
	}
}

// WithBanRate sets the hard limit (default 20/s, burst 20). Peers above it
// are banned for the ban duration, whether or not they solve proof of work.
func WithBanRate(perSecond float64, burst int) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if perSecond > 0 && burst > 0 {
			c.hardRate, c.hardBurst = perSecond, float64(burst)
		}
	}
}

// WithBanDuration sets how long a peer stays banned (default 1 minute).
func WithBanDuration(d time.Duration) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if d > 0 {
			c.banDuration = d
		}
	}
}

// WithPoW enables or disables proof-of-work challenges (default enabled).
func WithPoW(enabled bool) RateLimiterOption {
	return func(c *rateLimiterConfig) { c.pow = enabled }
}

// WithPoWHandshake makes the middleware always start the connection with a
// line the client can act on: "OK\n" when no work is needed, or a challenge
// followed by "OK\n" after a valid solution. Clients use [AnswerPoW]. Without
// it (the default) unthrottled connections see no extra bytes, which keeps
// existing wire protocols intact but means clients cannot tell in advance
// whether a challenge is coming.
func WithPoWHandshake(enabled bool) RateLimiterOption {
	return func(c *rateLimiterConfig) { c.powHandshake = enabled }
}

// WithPoWTimeout sets how long a client has to solve a challenge
// (default 10s).
func WithPoWTimeout(d time.Duration) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if d > 0 {
			c.powTimeout = d
		}
	}
}

// WithPoWDifficulty sets the difficulty range in leading zero bits
// (default 16..24). Difficulty starts at minBits, grows by one bit for every
// further challenge and decays back by one bit per decay interval.
func WithPoWDifficulty(minBits, maxBits int) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		minBits = min(max(minBits, 1), MaxPoWBits)
		c.minBits, c.maxBits = minBits, min(max(maxBits, minBits), MaxPoWBits)
	}
}

// WithDifficultyDecay sets how fast difficulty decays (default one bit per 10s).
func WithDifficultyDecay(d time.Duration) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if d > 0 {
			c.decay = d
		}
	}
}

// WithIPv6PrefixLen groups IPv6 peers by prefix (default /64, the usual
// allocation for a single host or subscriber). Use 128 to track addresses
// individually. IPv4 addresses are always tracked individually.
func WithIPv6PrefixLen(bits int) RateLimiterOption {
	return func(c *rateLimiterConfig) { c.v6PrefixLen = min(max(bits, 0), 128) }
}

// WithMaxTrackedPeers bounds memory (default 1,048,576 peers, about 64 MB).
// When a shard is full, new peers must solve proof of work at the maximum
// difficulty (or are rejected when PoW is disabled).
func WithMaxTrackedPeers(n int) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if n > 0 {
			c.maxPeers = n
		}
	}
}

// WithCleanupInterval sets how often stale peers are removed (default 1
// minute). 0 disables the background goroutine; call Cleanup yourself.
func WithCleanupInterval(d time.Duration) RateLimiterOption {
	return func(c *rateLimiterConfig) { c.cleanupInterval = max(d, 0) }
}

// WithClock injects a time source (for tests).
func WithClock(now func() time.Time) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if now != nil {
			c.now = now
		}
	}
}

// WithRateLimiterLogger sets a logger for bans (default: discard).
func WithRateLimiterLogger(l *slog.Logger) RateLimiterOption {
	return func(c *rateLimiterConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

type peerKey [16]byte

// peerState has no pointers, so large maps of it are not scanned by the GC.
type peerState struct {
	soft, hard  float64 // tokens
	last        int64   // unix ns of the last refill
	bannedUntil int64   // unix ns
	diffAt      int64   // unix ns of the last challenge; 0 = never
	difficulty  int32   // bits of the last challenge
}

type shard struct {
	mu sync.Mutex
	m  map[peerKey]peerState
	_  [48]byte // keep shards on separate cache lines
}

// RateLimiter limits connections per peer IP with two token buckets: above
// the soft rate a peer must solve adaptive proof of work, above the hard rate
// it is banned. State is split over 64 independently locked shards, so
// unrelated peers never contend.
//
// The limiter keys on conn.RemoteAddr(). Behind a TCP load balancer or proxy
// that is the proxy's address, which would rate-limit everyone together: use
// the PROXY protocol (for example github.com/pires/go-proxyproto as the
// listener) so RemoteAddr reports the real client.
type RateLimiter struct {
	cfg        rateLimiterConfig
	shards     [rateLimiterShards]shard
	perShard   int
	staleAfter int64

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewRateLimiter creates a limiter and, unless disabled with
// WithCleanupInterval(0), starts a cleanup goroutine; call Stop when done.
func NewRateLimiter(opts ...RateLimiterOption) *RateLimiter {
	cfg := rateLimiterConfig{
		softRate: 10, softBurst: 10,
		hardRate: 20, hardBurst: 20,
		banDuration:     time.Minute,
		pow:             true,
		powTimeout:      10 * time.Second,
		minBits:         DefaultPoWMinBits,
		maxBits:         DefaultPoWMaxBits,
		decay:           10 * time.Second,
		v6PrefixLen:     64,
		maxPeers:        1 << 20,
		cleanupInterval: time.Minute,
		now:             time.Now,
		logger:          slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	r := &RateLimiter{
		cfg:      cfg,
		perShard: max(cfg.maxPeers/rateLimiterShards, 1),
		stopCh:   make(chan struct{}),
	}
	refill := max(cfg.softBurst/cfg.softRate, cfg.hardBurst/cfg.hardRate)
	r.staleAfter = max(int64(refill*float64(time.Second)),
		int64(cfg.decay)*int64(cfg.maxBits-cfg.minBits+1))
	for i := range r.shards {
		r.shards[i].m = make(map[peerKey]peerState)
	}
	if cfg.cleanupInterval > 0 {
		r.wg.Go(r.cleanupLoop)
	}
	return r
}

// Stop stops the cleanup goroutine. It is idempotent.
func (r *RateLimiter) Stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
	r.wg.Wait()
}

func (r *RateLimiter) cleanupLoop() {
	t := time.NewTicker(r.cfg.cleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.Cleanup()
		case <-r.stopCh:
			return
		}
	}
}

// Cleanup removes peers that are not banned and whose buckets and difficulty
// have fully recovered. It returns the number of removed peers.
func (r *RateLimiter) Cleanup() int {
	now := r.cfg.now().UnixNano()
	removed := 0
	for i := range r.shards {
		s := &r.shards[i]
		s.mu.Lock()
		removed += r.pruneLocked(s, now)
		s.mu.Unlock()
	}
	return removed
}

func (r *RateLimiter) pruneLocked(s *shard, now int64) int {
	n := 0
	for k, st := range s.m {
		if st.bannedUntil <= now && now-st.last > r.staleAfter {
			delete(s.m, k)
			n++
		}
	}
	return n
}

// Len returns the number of tracked peers.
func (r *RateLimiter) Len() int {
	n := 0
	for i := range r.shards {
		s := &r.shards[i]
		s.mu.Lock()
		n += len(s.m)
		s.mu.Unlock()
	}
	return n
}

func (r *RateLimiter) key(addr netip.Addr) peerKey {
	addr = addr.Unmap()
	if addr.Is6() && r.cfg.v6PrefixLen < 128 {
		if p, err := addr.Prefix(r.cfg.v6PrefixLen); err == nil {
			addr = p.Addr()
		}
	}
	return addr.As16()
}

func shardIndex(k *peerKey) int {
	h := binary.LittleEndian.Uint64(k[:8]) ^ bits.RotateLeft64(binary.LittleEndian.Uint64(k[8:]), 31)
	h *= 0x9E3779B97F4A7C15
	return int(h >> (64 - shardBits))
}

// Check records a connection attempt from addr and returns the decision and,
// for RequirePoW, the difficulty in bits. It is safe for concurrent use and
// does not allocate for known peers.
func (r *RateLimiter) Check(addr netip.Addr) (Decision, int) {
	k := r.key(addr)
	s := &r.shards[shardIndex(&k)]
	now := r.cfg.now().UnixNano()

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.m[k]
	if !ok {
		if len(s.m) >= r.perShard && r.pruneLocked(s, now) == 0 {
			// Saturated: don't track, but make the newcomer pay.
			return RequirePoW, r.cfg.maxBits
		}
		st = peerState{soft: r.cfg.softBurst, hard: r.cfg.hardBurst, last: now}
	}
	if st.bannedUntil > now {
		return Deny, 0
	}

	elapsed := float64(now-st.last) / float64(time.Second)
	if elapsed > 0 {
		st.soft = min(r.cfg.softBurst, st.soft+elapsed*r.cfg.softRate)
		st.hard = min(r.cfg.hardBurst, st.hard+elapsed*r.cfg.hardRate)
		st.last = now
	}

	if st.hard < 1 {
		st.bannedUntil = now + int64(r.cfg.banDuration)
		st.soft, st.hard = r.cfg.softBurst, r.cfg.hardBurst
		s.m[k] = st
		r.cfg.logger.Info("tcp: peer banned", "peer", addr.String(), "for", r.cfg.banDuration)
		return Deny, 0
	}
	st.hard--

	if st.soft >= 1 {
		st.soft--
		s.m[k] = st
		return Allow, 0
	}

	next := r.cfg.minBits
	if st.diffAt != 0 {
		steps := int32((now - st.diffAt) / int64(r.cfg.decay))
		effective := max(int32(r.cfg.minBits-1), st.difficulty-steps)
		next = min(r.cfg.maxBits, int(effective)+1)
	}
	st.difficulty, st.diffAt = int32(next), now
	s.m[k] = st
	return RequirePoW, next
}

// Middleware returns server middleware that applies the limiter: banned peers
// are rejected with [ErrBanned]; peers over the soft rate get a PoW challenge
// (or [ErrRateLimited] when PoW is disabled). Connections without an IP
// remote address are rejected.
func (r *RateLimiter) Middleware() Middleware {
	return func(ctx context.Context, conn net.Conn) (net.Conn, error) {
		addr, ok := remoteIP(conn)
		if !ok {
			return nil, errors.Wrap(ErrRateLimited, "tcp: remote address is not an IP")
		}
		decision, difficulty := r.Check(addr)
		switch decision {
		case Deny:
			return nil, ErrBanned
		case RequirePoW:
			if !r.cfg.pow {
				return nil, ErrRateLimited
			}
			return r.challenge(ctx, conn, difficulty)
		}
		if r.cfg.powHandshake {
			if err := r.writeOK(conn); err != nil {
				return nil, err
			}
		}
		return conn, nil
	}
}

// RateLimitMiddleware returns limiter.Middleware().
//
// Deprecated: use (*RateLimiter).Middleware.
func RateLimitMiddleware(limiter *RateLimiter) Middleware { return limiter.Middleware() }

func (r *RateLimiter) challenge(ctx context.Context, conn net.Conn, difficulty int) (net.Conn, error) {
	ch, err := NewPoWChallenge(difficulty, r.cfg.powTimeout)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(ch.Expires); err != nil {
		return nil, wrapError("pow", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(aLongTimeAgo) })
	defer stop()

	if err := WritePoWChallenge(conn, ch); err != nil {
		return nil, wrapError("pow", err)
	}
	br := bufio.NewReaderSize(conn, maxPoWLine)
	nonce, err := ReadPoWSolution(br)
	if err != nil {
		return nil, errors.Join(ErrPoWFailed, err)
	}
	if !ch.Verify(nonce) {
		return nil, ErrPoWFailed
	}
	if r.cfg.powHandshake {
		if _, err := io.WriteString(conn, powOK+"\n"); err != nil {
			return nil, wrapError("pow", err)
		}
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, wrapError("pow", err)
	}
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

func (r *RateLimiter) writeOK(conn net.Conn) error {
	if err := conn.SetWriteDeadline(time.Now().Add(r.cfg.powTimeout)); err != nil {
		return wrapError("pow", err)
	}
	if _, err := io.WriteString(conn, powOK+"\n"); err != nil {
		return wrapError("pow", err)
	}
	return wrapErrorOrNil("pow", conn.SetWriteDeadline(time.Time{}))
}

func wrapErrorOrNil(op string, err error) error {
	if err == nil {
		return nil
	}
	return wrapError(op, err)
}

func remoteIP(conn net.Conn) (netip.Addr, bool) {
	switch a := conn.RemoteAddr().(type) {
	case *net.TCPAddr:
		ip, ok := netip.AddrFromSlice(a.IP)
		return ip, ok
	case nil:
		return netip.Addr{}, false
	default:
		ap, err := netip.ParseAddrPort(a.String())
		if err != nil {
			return netip.Addr{}, false
		}
		return ap.Addr(), true
	}
}
