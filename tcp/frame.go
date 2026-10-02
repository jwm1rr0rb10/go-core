package tcp

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
)

// FrameHeaderSize is the size of the big-endian uint32 length prefix.
const FrameHeaderSize = 4

// DefaultMaxFrameSize is the frame limit used when maxSize <= 0 (4 MiB).
const DefaultMaxFrameSize = 4 << 20

// coalesceLimit is the largest payload copied into a pooled buffer so that
// header and payload reach the writer in a single Write call.
const coalesceLimit = 32 << 10

var frameBufPool = sync.Pool{New: func() any {
	b := make([]byte, 0, 4096)
	return &b
}}

// WriteFrame writes payload prefixed with its length as a 4-byte big-endian
// integer, always in a way that produces one packet/TLS record where possible:
//
//   - payloads up to 32 KiB: header and payload are copied into a pooled
//     buffer and written with one Write (no allocation in steady state);
//   - larger payloads on a *net.TCPConn: one writev syscall, no copy;
//   - larger payloads on other writers: two Writes.
//
// Payloads larger than maxSize (DefaultMaxFrameSize when maxSize <= 0) are
// rejected with [ErrFrameTooLarge].
func WriteFrame(w io.Writer, payload []byte, maxSize int) error {
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}
	if len(payload) > maxSize || uint64(len(payload)) > uint64(^uint32(0)) {
		return ErrFrameTooLarge
	}
	if tc, ok := w.(*net.TCPConn); ok && len(payload) > coalesceLimit {
		var hdr [FrameHeaderSize]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
		bufs := net.Buffers{hdr[:], payload}
		_, err := bufs.WriteTo(tc)
		return err
	}
	if len(payload) <= coalesceLimit {
		bp := frameBufPool.Get().(*[]byte)
		b := binary.BigEndian.AppendUint32((*bp)[:0], uint32(len(payload)))
		b = append(b, payload...)
		_, err := w.Write(b)
		if cap(b) <= 64<<10 {
			*bp = b[:0]
			frameBufPool.Put(bp)
		}
		return err
	}
	bp := frameBufPool.Get().(*[]byte)
	hdr := binary.BigEndian.AppendUint32((*bp)[:0], uint32(len(payload)))
	_, err := w.Write(hdr)
	frameBufPool.Put(bp)
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

// ReadFrame reads one length-prefixed frame from r. It reuses buf when its
// capacity is large enough, so passing the previous result back in makes a
// read loop allocation-free:
//
//	var buf []byte
//	for {
//		buf, err = tcp.ReadFrame(r, buf[:0], 0)
//		...
//	}
//
// The length is validated against maxSize (DefaultMaxFrameSize when <= 0)
// before any allocation, so a malicious peer cannot force a huge allocation.
func ReadFrame(r io.Reader, buf []byte, maxSize int) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}
	// Read the header into buf when it has room, so it does not escape to the
	// heap through the io.Reader interface.
	if cap(buf) < FrameHeaderSize {
		buf = make([]byte, FrameHeaderSize, 64)
	}
	hdr := buf[:FrameHeaderSize]
	if _, err := io.ReadFull(r, hdr); err != nil {
		return buf[:0], err
	}
	n := binary.BigEndian.Uint32(hdr)
	if uint64(n) > uint64(maxSize) {
		return buf[:0], ErrFrameTooLarge
	}
	if cap(buf) < int(n) {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	if _, err := io.ReadFull(r, buf); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return buf[:0], err
	}
	return buf, nil
}
