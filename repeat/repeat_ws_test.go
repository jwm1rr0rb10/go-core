package repeat_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/jwm1rr0rb10/go-core/repeat"
)

func TestConnectWithRetry(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	real := repeat.Dialer(nil, wsURL, nil)
	var attempts, retries atomic.Int32
	dial := func(ctx context.Context) (*websocket.Conn, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("dial failed")
		}
		return real(ctx)
	}

	conn, err := repeat.ConnectWithRetry(context.Background(), dial,
		repeat.WithConstantDelay(time.Millisecond),
		repeat.WithOnRetry(func(repeat.RetryEvent) { retries.Add(1) }))
	if err != nil {
		t.Fatalf("connect with retry: %v", err)
	}
	_ = conn.Close()
	if attempts.Load() != 2 || retries.Load() != 1 {
		t.Fatalf("attempts=%d retries=%d", attempts.Load(), retries.Load())
	}
}

func TestConnectWithRetryFails(t *testing.T) {
	_, err := repeat.ConnectWithRetry(context.Background(),
		repeat.Dialer(&websocket.Dialer{HandshakeTimeout: time.Second}, "ws://127.0.0.1:1", nil),
		repeat.WithConstantDelay(time.Millisecond), repeat.WithMaxAttempts(2))
	var re *repeat.Error
	if !errors.As(err, &re) || re.Attempts != 2 || !strings.HasPrefix(err.Error(), "repeat: websocket connect") {
		t.Fatalf("err=%v", err)
	}
}
