package tcp

import (
	"testing"
	"time"
)

func TestPoWInvalidSolutionRejected(t *testing.T) {
	s, _ := powServer(t, WithPoWDifficulty(20, 20))
	echoCheck(t, rawDial(t, s), "ok")

	conn := rawDial(t, s)
	if _, err := readLineUnbuffered(conn); err != nil {
		t.Fatal(err)
	}
	if err := WritePoWSolution(conn, "definitely-wrong"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection should be closed after an invalid solution")
	}
	waitFor(t, time.Second, func() bool { return s.Stats().RejectedConnections == 1 })
}

func TestPoWGarbageRejected(t *testing.T) {
	s, _ := powServer(t)
	echoCheck(t, rawDial(t, s), "ok")
	conn := rawDial(t, s)
	_, _ = readLineUnbuffered(conn)
	_, _ = conn.Write(make([]byte, 1024)) // no newline, longer than the line limit
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection should be closed")
	}
}

func TestPoWDisabledRejects(t *testing.T) {
	s, _ := powServer(t, WithPoW(false))
	echoCheck(t, rawDial(t, s), "ok")
	if _, err := rawDial(t, s).Read(make([]byte, 1)); err == nil {
		t.Fatal("over-limit connection should be closed when PoW is disabled")
	}
}

func TestBannedPeerRejected(t *testing.T) {
	s, rl := powServer(t, WithBanRate(0.001, 1), WithBanDuration(time.Hour))
	echoCheck(t, rawDial(t, s), "ok")
	if _, err := rawDial(t, s).Read(make([]byte, 1)); err == nil {
		t.Fatal("banned peer should be closed")
	}
	if d, _ := rl.Check(ip1Loopback()); d != Deny {
		t.Fatalf("expected deny, got %v", d)
	}
}
