package repeat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastHTTP() []Option {
	return []Option{WithConstantDelay(time.Millisecond), WithMaxAttempts(3)}
}

// flaky answers with the given statuses in order, then 200.
func flaky(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32, *[]string) {
	t.Helper()
	var calls atomic.Int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		n := int(calls.Add(1))
		if n <= len(statuses) {
			w.WriteHeader(statuses[n-1])
			_, _ = io.WriteString(w, "fail")
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &bodies
}

func TestClientRetriesRetryableStatus(t *testing.T) {
	srv, calls, _ := flaky(t, http.StatusServiceUnavailable, http.StatusBadGateway)
	resp, err := NewClient(nil, fastHTTP()...).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "ok" || calls.Load() != 3 {
		t.Fatalf("status=%d body=%q calls=%d", resp.StatusCode, body, calls.Load())
	}
}

func TestClientDoesNotRetry500ByDefault(t *testing.T) {
	srv, calls, _ := flaky(t, http.StatusInternalServerError)
	resp, err := NewClient(nil, fastHTTP()...).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 || calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, calls.Load())
	}
}

func TestClientReturnsLastResponseWhenExhausted(t *testing.T) {
	srv, calls, _ := flaky(t, 503, 503, 503, 503)
	resp, err := NewClient(nil, fastHTTP()...).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 503 || string(body) != "fail" || calls.Load() != 3 {
		t.Fatalf("status=%d body=%q calls=%d", resp.StatusCode, body, calls.Load())
	}
}

func TestClientRewindsBody(t *testing.T) {
	srv, calls, bodies := flaky(t, 503)
	req, _ := http.NewRequest(http.MethodPut, srv.URL, strings.NewReader("payload"))
	resp, err := NewClient(nil, fastHTTP()...).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 2 || (*bodies)[0] != "payload" || (*bodies)[1] != "payload" {
		t.Fatalf("calls=%d bodies=%q", calls.Load(), *bodies)
	}
}

func TestClientDoesNotRetryPOSTByDefault(t *testing.T) {
	srv, calls, _ := flaky(t, 503)
	resp, err := NewClient(nil, fastHTTP()...).Post(srv.URL, "text/plain", bytes.NewBufferString("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestClientRetriesPOSTWithIdempotencyKey(t *testing.T) {
	srv, calls, bodies := flaky(t, 503)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("x")))
	req.Header.Set("Idempotency-Key", "k1")
	resp, err := NewClient(nil, fastHTTP()...).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 2 || (*bodies)[1] != "x" {
		t.Fatalf("calls=%d bodies=%q", calls.Load(), *bodies)
	}
}

func TestTransportRetryNonIdempotent(t *testing.T) {
	srv, calls, _ := flaky(t, 503)
	tr := NewTransport(nil, fastHTTP()...)
	tr.RetryNonIdempotent = true
	resp, err := (&http.Client{Transport: tr}).Post(srv.URL, "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestTransportNonRewindableBodySentOnce(t *testing.T) {
	srv, calls, _ := flaky(t, 503)
	req, _ := http.NewRequest(http.MethodPut, srv.URL, io.NopCloser(strings.NewReader("x")))
	resp, err := NewClient(nil, fastHTTP()...).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 1 || resp.StatusCode != 503 {
		t.Fatalf("calls=%d status=%d", calls.Load(), resp.StatusCode)
	}
}

func TestTransportCustomRetryStatus(t *testing.T) {
	srv, calls, _ := flaky(t, 500)
	tr := NewTransport(nil, fastHTTP()...)
	tr.RetryStatus = func(code int) bool { return code >= 500 }
	resp, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 2 || resp.StatusCode != 200 {
		t.Fatalf("calls=%d status=%d", calls.Load(), resp.StatusCode)
	}
}

func TestTransportHonorsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	var delay time.Duration
	c := NewClient(nil, WithConstantDelay(time.Millisecond), WithOnRetry(func(e RetryEvent) {
		delay = e.Delay
		var se *StatusError
		if !errors.As(e.Err, &se) || se.StatusCode != 429 || se.RetryAfter() != time.Second {
			t.Errorf("event err %v", e.Err)
		}
	}))
	start := time.Now()
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if delay != time.Second || time.Since(start) < time.Second {
		t.Fatalf("delay=%v elapsed=%v", delay, time.Since(start))
	}
}

func TestTransportRetriesNetworkErrors(t *testing.T) {
	var calls atomic.Int32
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) < 3 {
			return nil, errors.New("connection reset")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	})
	resp, err := (&http.Client{Transport: NewTransport(base, fastHTTP()...)}).Get("http://example.invalid")
	if err != nil || resp.StatusCode != 200 || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestTransportNetworkErrorExhausted(t *testing.T) {
	base := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })
	_, err := (&http.Client{Transport: NewTransport(base, fastHTTP()...)}).Get("http://example.invalid")
	var re *Error
	if !errors.As(err, &re) || re.Attempts != 3 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v", err)
	}
}

func TestTransportStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		cancel()
		return nil, context.Canceled
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	_, err := NewTransport(base, fastHTTP()...).RoundTrip(req)
	if calls.Load() != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestTransportInvalidConfig(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	if _, err := NewTransport(nil, WithMaxAttempts(0)).RoundTrip(req); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v", err)
	}
}

func TestTransportDrainsDiscardedBodies(t *testing.T) {
	var closed atomic.Int32
	var calls atomic.Int32
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		code := 503
		if calls.Add(1) == 3 {
			code = 200
		}
		return &http.Response{StatusCode: code, Header: http.Header{},
			Body: &trackBody{Reader: strings.NewReader("x"), closed: &closed}, Request: r}, nil
	})
	resp, err := NewTransport(base, fastHTTP()...).RoundTrip(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("err=%v", err)
	}
	if closed.Load() != 2 {
		t.Fatalf("closed=%d, want 2 discarded bodies closed", closed.Load())
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"5":                             5 * time.Second,
		"-1":                            0,
		"junk":                          0,
		"999999999":                     24 * time.Hour,
		"Thu, 01 Jan 2026 00:00:10 GMT": 10 * time.Second,
		"Wed, 31 Dec 2025 23:59:00 GMT": 0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b *trackBody) Close() error { b.closed.Add(1); return nil }
