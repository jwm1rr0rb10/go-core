package tcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadPoWSolutionKeepsPipelinedBytes(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("SOLUTION nonce=abc\r\nAPPDATA"), 16)
	nonce, err := ReadPoWSolution(br)
	if err != nil || nonce != "abc" {
		t.Fatalf("nonce %q err %v", nonce, err)
	}
	rest, _ := io.ReadAll(br)
	if string(rest) != "APPDATA" {
		t.Fatalf("pipelined data lost: %q", rest)
	}
}

func TestReadPoWSolutionErrors(t *testing.T) {
	for _, in := range []string{
		"NONCE abc\n",
		"SOLUTION nonce=\n",
		"SOLUTION nonce=" + strings.Repeat("a", maxNonceLen+1) + "\n",
		strings.Repeat("x", maxPoWLine+10) + "\n",
	} {
		if _, err := ReadPoWSolution(bufio.NewReaderSize(strings.NewReader(in), 16)); !errors.Is(err, ErrPoWFailed) {
			t.Errorf("%.20q...: %v", in, err)
		}
	}
	if _, err := ReadPoWSolution(bufio.NewReader(strings.NewReader("SOLUTION"))); !errors.Is(err, io.EOF) {
		t.Fatalf("truncated: %v", err)
	}
}

func TestReadLineUnbuffered(t *testing.T) {
	r := strings.NewReader("OK\r\nNEXT")
	line, err := readLineUnbuffered(r)
	if err != nil || line != "OK" {
		t.Fatalf("%q %v", line, err)
	}
	rest, _ := io.ReadAll(r)
	if string(rest) != "NEXT" {
		t.Fatalf("over-read: %q", rest)
	}
	if _, err := readLineUnbuffered(strings.NewReader(strings.Repeat("a", maxPoWLine+1))); !errors.Is(err, ErrPoWFailed) {
		t.Fatalf("long line: %v", err)
	}
}

func TestSolvePoWCancel(t *testing.T) {
	ch, _ := NewPoWChallenge(MaxPoWBits, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := SolvePoW(ctx, ch); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("solver did not stop promptly")
	}
	if _, err := SolvePoW(context.Background(), nil); !errors.Is(err, ErrPoWFailed) {
		t.Fatal("nil challenge")
	}
}
