//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package tcp

import (
	"crypto/tls"
	"net"
	"syscall"

	"github.com/jwm1rr0rb10/go-errors"
)

// connAlive peeks at the socket without consuming data or blocking.
// Go sockets are non-blocking, so recv(MSG_PEEK) returns immediately:
//
//   - EAGAIN: nothing to read, the peer is still there -> alive
//   - 0 bytes: orderly shutdown (FIN) by the peer -> dead
//   - >0 bytes: unsolicited data on an idle pooled connection, which means
//     the protocol is out of sync (or a TLS close_notify) -> dead
//   - any other error (ECONNRESET, ...) -> dead
//
// Connections that do not expose a file descriptor (net.Pipe, custom
// wrappers) are assumed alive.
func connAlive(c net.Conn) bool {
	if tc, ok := c.(*tls.Conn); ok {
		c = tc.NetConn()
	}
	sc, ok := c.(syscall.Conn)
	if !ok {
		return true
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return false
	}
	alive := false
	var buf [1]byte
	err = raw.Read(func(fd uintptr) bool {
		_, _, rerr := syscall.Recvfrom(int(fd), buf[:], syscall.MSG_PEEK)
		alive = errors.Is(rerr, syscall.EAGAIN) || errors.Is(rerr, syscall.EWOULDBLOCK)
		return true // never wait for readiness
	})
	return err == nil && alive
}
