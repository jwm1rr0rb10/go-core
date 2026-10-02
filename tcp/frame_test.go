package tcp

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msgs := [][]byte{[]byte("a"), {}, bytes.Repeat([]byte("z"), 1000)}
	for _, m := range msgs {
		if err := WriteFrame(&buf, m, 0); err != nil {
			t.Fatal(err)
		}
	}
	var out []byte
	for _, m := range msgs {
		var err error
		out, err = ReadFrame(&buf, out[:0], 0)
		if err != nil || !bytes.Equal(out, m) {
			t.Fatalf("got %q %v", out, err)
		}
	}
	if _, err := ReadFrame(&buf, nil, 0); !errors.Is(err, io.EOF) {
		t.Fatalf("clean EOF expected, got %v", err)
	}
}

func TestFrameBufferReuse(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, []byte("hello"), 0)
	scratch := make([]byte, 0, 64)
	out, err := ReadFrame(&buf, scratch, 0)
	if err != nil || &out[0] != &scratch[:1][0] {
		t.Fatal("buffer was not reused")
	}
}

func TestFrameLimits(t *testing.T) {
	if err := WriteFrame(io.Discard, make([]byte, 11), 10); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("write: %v", err)
	}
	var buf bytes.Buffer
	_ = WriteFrame(&buf, make([]byte, 100), 0)
	if _, err := ReadFrame(&buf, nil, 10); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("read: %v", err)
	}
	// A hostile 4 GiB length must not allocate.
	huge := bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff})
	if _, err := ReadFrame(huge, nil, 0); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("huge: %v", err)
	}
}

func TestFrameTruncated(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, []byte("hello"), 0)
	trunc := bytes.NewReader(buf.Bytes()[:6])
	if _, err := ReadFrame(trunc, nil, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0, 0}), nil, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short header: %v", err)
	}
}
