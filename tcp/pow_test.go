package tcp

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewPoWChallengeClamps(t *testing.T) {
	for in, want := range map[int]int32{-3: 1, 0: 1, 12: 12, 99: MaxPoWBits} {
		ch, err := NewPoWChallenge(in, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch.Difficulty != want || len(ch.Prefix) != 32 || !ch.Expires.IsZero() {
			t.Fatalf("difficulty %d: %+v", in, ch)
		}
	}
	a, _ := NewPoWChallenge(4, time.Minute)
	b, _ := NewPoWChallenge(4, time.Minute)
	if a.Prefix == b.Prefix {
		t.Fatal("prefixes must be random")
	}
	if a.Expires.IsZero() {
		t.Fatal("expiry not set")
	}
}

func TestSolveAndVerify(t *testing.T) {
	ch, _ := NewPoWChallenge(12, time.Minute)
	nonce, err := SolvePoW(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Verify(nonce) {
		t.Fatal("solution rejected")
	}
	for _, bad := range []string{"", strings.Repeat("a", maxNonceLen+1)} {
		if ch.Verify(bad) {
			t.Fatalf("nonce %q accepted", bad)
		}
	}
	var nilCh *PoWChallenge
	if nilCh.Verify(nonce) {
		t.Fatal("nil challenge verified")
	}
}

func TestVerifyExpired(t *testing.T) {
	ch, _ := NewPoWChallenge(1, time.Minute)
	nonce, _ := SolvePoW(context.Background(), ch)
	ch.Expires = time.Now().Add(-time.Second)
	if ch.Verify(nonce) {
		t.Fatal("expired challenge verified")
	}
	if _, err := SolvePoW(context.Background(), ch); !errors.Is(err, ErrPoWFailed) {
		t.Fatalf("solving expired challenge: %v", err)
	}
}

func TestLeadingZeroBits(t *testing.T) {
	var h [32]byte
	if leadingZeroBits(&h) != 256 {
		t.Fatal("all zero")
	}
	h[0] = 0x80
	if leadingZeroBits(&h) != 0 {
		t.Fatal("msb set")
	}
	h[0] = 0
	h[9] = 0x10 // byte 9 → 72 bits + 3
	if got := leadingZeroBits(&h); got != 75 {
		t.Fatalf("got %d", got)
	}
}

func TestChallengeWireRoundTrip(t *testing.T) {
	ch, _ := NewPoWChallenge(20, time.Minute)
	var buf bytes.Buffer
	if err := WritePoWChallenge(&buf, ch); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "POW prefix=") || !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("wire format: %q", buf.String())
	}
	got, err := ParsePoWChallenge(buf.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != ch.Prefix || got.Difficulty != ch.Difficulty || got.Expires.Unix() != ch.Expires.Unix() {
		t.Fatalf("round trip: %+v vs %+v", got, ch)
	}
	if err := WritePoWChallenge(&buf, nil); err == nil {
		t.Fatal("nil challenge should fail")
	}
}

func TestParsePoWChallengeErrors(t *testing.T) {
	for _, line := range []string{
		"",
		"HELLO",
		"POW difficulty=4",
		"POW prefix=ab difficulty=0",
		"POW prefix=ab difficulty=99",
		"POW prefix=ab difficulty=x",
		"POW prefix=ab difficulty=4 expires=soon",
		"POW prefix=" + strings.Repeat("a", 65) + " difficulty=4",
	} {
		if _, err := ParsePoWChallenge(line); !errors.Is(err, ErrPoWFailed) {
			t.Errorf("%q: %v", line, err)
		}
	}
	ch, err := ParsePoWChallenge("POW prefix=abcd difficulty=3 expires=0\r\n")
	if err != nil || !ch.Expires.IsZero() {
		t.Fatalf("expires=0 means none: %+v %v", ch, err)
	}
}
