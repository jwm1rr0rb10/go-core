package repeat

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

// DialFunc dials a WebSocket connection.
type DialFunc func(ctx context.Context) (*websocket.Conn, error)

// Dialer returns a [DialFunc] that dials url with d (nil means
// websocket.DefaultDialer) and header. A failed handshake response body is
// closed.
func Dialer(d *websocket.Dialer, url string, header http.Header) DialFunc {
	if d == nil {
		d = websocket.DefaultDialer
	}
	return func(ctx context.Context) (*websocket.Conn, error) {
		conn, resp, err := d.DialContext(ctx, url, header)
		if err != nil && resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}
}

// ConnectWithRetry dials until a connection is established or the retry
// policy gives up. Use [WithOnRetry] to log failed attempts.
func ConnectWithRetry(ctx context.Context, dial DialFunc, opts ...Option) (*websocket.Conn, error) {
	conn, err := Do(ctx, func(ctx context.Context) (*websocket.Conn, error) {
		return dial(ctx)
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("repeat: websocket connect: %w", err)
	}
	return conn, nil
}
