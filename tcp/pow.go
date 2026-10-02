package tcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/bits"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

// Proof-of-work limits. Difficulty is the number of leading zero bits of
// SHA-256(prefix || nonce); every extra bit doubles the expected client work
// (16 bits ≈ 65 thousand hashes, 24 bits ≈ 16.7 million).
const (
	DefaultPoWMinBits = 16
	DefaultPoWMaxBits = 24
	MaxPoWBits        = 40

	powPrefixBytes = 16
	powPrefixLen   = powPrefixBytes * 2 // hex
	maxNonceLen    = 64
	maxPoWLine     = 256
)

const (
	powChallengeTag = "POW "
	powSolutionTag  = "SOLUTION nonce="
	powOK           = "OK"
)

// PoWChallenge is a proof-of-work puzzle. The prefix is 128 random bits, so a
// solution is bound to the one challenge (and connection) it was issued for
// and cannot be replayed; Expires bounds how long it can be worked on.
type PoWChallenge struct {
	Prefix     string    // 32 hex characters
	Difficulty int32     // required leading zero bits
	Expires    time.Time // zero means no expiry
}

// NewPoWChallenge creates a challenge with difficulty bits (clamped to
// [1, MaxPoWBits]) that expires after ttl (0 = never).
func NewPoWChallenge(difficulty int, ttl time.Duration) (*PoWChallenge, error) {
	var raw [powPrefixBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, errors.Wrap(err, "tcp: pow prefix")
	}
	ch := &PoWChallenge{
		Prefix:     hex.EncodeToString(raw[:]),
		Difficulty: int32(min(max(difficulty, 1), MaxPoWBits)),
	}
	if ttl > 0 {
		ch.Expires = time.Now().Add(ttl)
	}
	return ch, nil
}

// Verify reports whether nonce solves the challenge and the challenge has not
// expired. It does not allocate.
func (c *PoWChallenge) Verify(nonce string) bool {
	if c == nil || (!c.Expires.IsZero() && time.Now().After(c.Expires)) {
		return false
	}
	return verifyPoW(c.Prefix, nonce, int(c.Difficulty))
}

func verifyPoW(prefix, nonce string, difficulty int) bool {
	if nonce == "" || len(nonce) > maxNonceLen || len(prefix) > powPrefixLen*2 {
		return false
	}
	var buf [powPrefixLen*2 + maxNonceLen]byte
	n := copy(buf[:], prefix)
	n += copy(buf[n:], nonce)
	sum := sha256.Sum256(buf[:n])
	return leadingZeroBits(&sum) >= difficulty
}

func leadingZeroBits(h *[32]byte) int {
	for i := 0; i < 32; i += 8 {
		if w := binary.BigEndian.Uint64(h[i:]); w != 0 {
			return i*8 + bits.LeadingZeros64(w)
		}
	}
	return 256
}

// SolvePoW finds a nonce for ch using all CPUs. It returns ctx.Err() if ctx
// is done first, or [ErrPoWFailed] once the challenge expires.
func SolvePoW(ctx context.Context, ch *PoWChallenge) (string, error) {
	if ch == nil || len(ch.Prefix) > powPrefixLen*2 {
		return "", ErrPoWFailed
	}
	if !ch.Expires.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, ch.Expires)
		defer cancel()
	}
	workers := runtime.GOMAXPROCS(0)
	var (
		found  atomic.Bool
		result string
		once   sync.Once
		wg     sync.WaitGroup
	)
	for w := range workers {
		wg.Go(func() {
			var buf [powPrefixLen*2 + 20]byte
			p := copy(buf[:], ch.Prefix)
			for i := uint64(w); ; i += uint64(workers) {
				if i&0xffff < uint64(workers) && (found.Load() || ctx.Err() != nil) {
					return
				}
				end := len(strconv.AppendUint(buf[p:p], i, 36)) + p
				sum := sha256.Sum256(buf[:end])
				if leadingZeroBits(&sum) >= int(ch.Difficulty) {
					once.Do(func() { result = string(buf[p:end]) })
					found.Store(true)
					return
				}
			}
		})
	}
	wg.Wait()
	if found.Load() {
		return result, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !ch.Expires.IsZero() && time.Now().After(ch.Expires) {
		return "", ErrPoWFailed
	}
	return "", ctx.Err()
}

// WritePoWChallenge writes "POW prefix=<hex> difficulty=<bits> expires=<unix>\n".
func WritePoWChallenge(w io.Writer, ch *PoWChallenge) error {
	if ch == nil {
		return errors.New("tcp: pow challenge is nil")
	}
	var exp int64
	if !ch.Expires.IsZero() {
		exp = ch.Expires.Unix()
	}
	b := make([]byte, 0, 96)
	b = append(b, powChallengeTag+"prefix="...)
	b = append(b, ch.Prefix...)
	b = append(b, " difficulty="...)
	b = strconv.AppendInt(b, int64(ch.Difficulty), 10)
	b = append(b, " expires="...)
	b = strconv.AppendInt(b, exp, 10)
	b = append(b, '\n')
	_, err := w.Write(b)
	return err
}

// ParsePoWChallenge parses a challenge line (with or without the newline).
func ParsePoWChallenge(line string) (*PoWChallenge, error) {
	line = strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(line, powChallengeTag)
	if !ok {
		return nil, errors.Wrap(ErrPoWFailed, "tcp: not a pow challenge")
	}
	ch := &PoWChallenge{}
	for _, field := range strings.Fields(rest) {
		k, v, _ := strings.Cut(field, "=")
		switch k {
		case "prefix":
			ch.Prefix = v
		case "difficulty":
			d, err := strconv.ParseInt(v, 10, 32)
			if err != nil || d < 1 || d > MaxPoWBits {
				return nil, errors.Wrap(ErrPoWFailed, "tcp: bad pow difficulty")
			}
			ch.Difficulty = int32(d)
		case "expires":
			e, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, errors.Wrap(ErrPoWFailed, "tcp: bad pow expiry")
			}
			if e > 0 {
				ch.Expires = time.Unix(e, 0)
			}
		}
	}
	if ch.Prefix == "" || len(ch.Prefix) > powPrefixLen*2 || ch.Difficulty == 0 {
		return nil, errors.Wrap(ErrPoWFailed, "tcp: incomplete pow challenge")
	}
	return ch, nil
}

// WritePoWSolution writes "SOLUTION nonce=<nonce>\n".
func WritePoWSolution(w io.Writer, nonce string) error {
	_, err := io.WriteString(w, powSolutionTag+nonce+"\n")
	return err
}

// ReadPoWSolution reads a solution line from r and returns the nonce. Lines
// longer than 256 bytes are rejected. r is a *bufio.Reader so bytes the
// client sent after the solution are not lost; keep reading from it.
func ReadPoWSolution(r *bufio.Reader) (string, error) {
	line, err := readLimitedLine(r)
	if err != nil {
		return "", err
	}
	nonce, ok := strings.CutPrefix(line, powSolutionTag)
	if !ok || nonce == "" || len(nonce) > maxNonceLen {
		return "", errors.Wrap(ErrPoWFailed, "tcp: invalid solution format")
	}
	return nonce, nil
}

func readLimitedLine(r *bufio.Reader) (string, error) {
	var b []byte
	for {
		chunk, err := r.ReadSlice('\n')
		b = append(b, chunk...)
		if len(b) > maxPoWLine {
			return "", errors.Wrap(ErrPoWFailed, "tcp: pow line too long")
		}
		if err == nil {
			return strings.TrimRight(string(b), "\r\n"), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", err
		}
	}
}

// readLineUnbuffered reads one line byte by byte so nothing after the
// newline is consumed. It is used on the client side where the stream that
// follows belongs to the application.
func readLineUnbuffered(r io.Reader) (string, error) {
	var (
		buf  [maxPoWLine]byte
		one  [1]byte
		size int
	)
	for size < len(buf) {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return "", err
		}
		if one[0] == '\n' {
			return strings.TrimRight(string(buf[:size]), "\r"), nil
		}
		buf[size] = one[0]
		size++
	}
	return "", errors.Wrap(ErrPoWFailed, "tcp: pow line too long")
}

// AnswerPoW performs the client side of the rate limiter handshake (see
// [WithPoWHandshake]): it reads the server's first line and, if it is a
// challenge, solves it, sends the solution and waits for the final "OK".
// It never reads past the handshake, so the connection is ready for the
// application protocol afterwards. ctx bounds the whole exchange.
func AnswerPoW(ctx context.Context, conn net.Conn) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
		defer conn.SetDeadline(time.Time{})
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(aLongTimeAgo) })
	defer stop()

	line, err := readLineUnbuffered(conn)
	if err != nil {
		return wrapError("pow", err)
	}
	if line == powOK {
		return nil
	}
	ch, err := ParsePoWChallenge(line)
	if err != nil {
		return err
	}
	nonce, err := SolvePoW(ctx, ch)
	if err != nil {
		return err
	}
	if err := WritePoWSolution(conn, nonce); err != nil {
		return wrapError("pow", err)
	}
	line, err = readLineUnbuffered(conn)
	if err != nil {
		return errors.Wrap(ErrPoWFailed, "tcp: solution rejected")
	}
	if line != powOK {
		return errors.Wrap(ErrPoWFailed, "tcp: unexpected reply "+strconv.Quote(line))
	}
	return nil
}

// bufferedConn serves reads from the bufio.Reader used during the PoW
// exchange so bytes the client pipelined after its solution are not lost.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// NetConn returns the wrapped connection.
func (c *bufferedConn) NetConn() net.Conn { return c.Conn }
