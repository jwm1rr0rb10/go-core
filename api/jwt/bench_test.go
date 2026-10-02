package jwt_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkGeneratePair(b *testing.B) {
	h := newHelper(b)
	for b.Loop() {
		if _, err := h.GeneratePair("user", 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAccess(b *testing.B) {
	h := newHelper(b)
	pair, _ := h.GeneratePair("user", 1)
	for b.Loop() {
		if _, err := h.ParseAccess(pair.AccessToken); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAccessParallel(b *testing.B) {
	h := newHelper(b)
	pair, _ := h.GeneratePair("user", 1)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := h.ParseAccess(pair.AccessToken); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkHTTPMiddleware(b *testing.B) {
	h := newHelper(b)
	pair, _ := h.GeneratePair("user", 1)
	mw := h.HTTPMiddleware()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	for b.Loop() {
		mw.ServeHTTP(w, req)
	}
}
