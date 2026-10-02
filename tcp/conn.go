package tcp

import (
	"net"
	"sync/atomic"
	"time"
)

// serverConn wraps every accepted connection. It counts bytes, records the
// last activity time and, when an idle timeout is configured, keeps sliding
// read/write deadlines so a connection is closed only after it has been
// silent for the whole timeout.
//
// Hot-path cost is two uncontended atomic adds, one time.Now and, at most once
// per idle/16, a deadline update. Counters live on the connection itself, so
// thousands of busy connections never contend on a shared cache line.
type serverConn struct {
	net.Conn

	idle    time.Duration
	refresh int64 // idle/16 in nanoseconds: minimum gap between deadline updates

	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
	lastActivity atomic.Int64 // unix nanoseconds

	// Deadline bookkeeping (unix nanoseconds of the last automatic update) and
	// manual-override flags: once the handler sets its own deadline for a
	// direction, automatic refreshing stops until it clears it with a zero time.
	readSetAt   atomic.Int64
	writeSetAt  atomic.Int64
	manualRead  atomic.Bool
	manualWrite atomic.Bool
}

func newServerConn(c net.Conn, idle time.Duration, now time.Time) *serverConn {
	sc := &serverConn{Conn: c, idle: idle, refresh: int64(idle / 16)}
	sc.lastActivity.Store(now.UnixNano())
	return sc
}

// Read implements net.Conn.
func (c *serverConn) Read(b []byte) (int, error) {
	if c.idle > 0 && !c.manualRead.Load() {
		c.slide(&c.readSetAt, c.Conn.SetReadDeadline)
	}
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.bytesRead.Add(int64(n))
		c.lastActivity.Store(time.Now().UnixNano())
	}
	return n, err
}

// Write implements net.Conn.
func (c *serverConn) Write(b []byte) (int, error) {
	if c.idle > 0 && !c.manualWrite.Load() {
		c.slide(&c.writeSetAt, c.Conn.SetWriteDeadline)
	}
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.bytesWritten.Add(int64(n))
		c.lastActivity.Store(time.Now().UnixNano())
	}
	return n, err
}

func (c *serverConn) slide(setAt *atomic.Int64, set func(time.Time) error) {
	now := time.Now()
	ns := now.UnixNano()
	if ns-setAt.Load() < c.refresh {
		return
	}
	setAt.Store(ns)
	_ = set(now.Add(c.idle))
}

// SetDeadline implements net.Conn. A non-zero deadline disables automatic
// idle refreshing for both directions; a zero time re-enables it.
func (c *serverConn) SetDeadline(t time.Time) error {
	c.manualRead.Store(!t.IsZero())
	c.manualWrite.Store(!t.IsZero())
	c.readSetAt.Store(0)
	c.writeSetAt.Store(0)
	return c.Conn.SetDeadline(t)
}

// SetReadDeadline implements net.Conn; see SetDeadline.
func (c *serverConn) SetReadDeadline(t time.Time) error {
	c.manualRead.Store(!t.IsZero())
	c.readSetAt.Store(0)
	return c.Conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn; see SetDeadline.
func (c *serverConn) SetWriteDeadline(t time.Time) error {
	c.manualWrite.Store(!t.IsZero())
	c.writeSetAt.Store(0)
	return c.Conn.SetWriteDeadline(t)
}

// NetConn returns the wrapped connection (for example a *tls.Conn), mirroring
// (*tls.Conn).NetConn.
func (c *serverConn) NetConn() net.Conn { return c.Conn }
