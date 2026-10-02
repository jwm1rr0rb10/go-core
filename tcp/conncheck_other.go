//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package tcp

import "net"

// connAlive cannot peek at sockets portably on this platform; connections
// are assumed alive and failures surface on first use instead.
func connAlive(net.Conn) bool { return true }
