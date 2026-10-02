package jwt_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corejwt "github.com/jwm1rr0rb10/go-core/api/jwt"
)

func okHandler(t *testing.T, wantUser string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := corejwt.GetUserID(r.Context())
		if err != nil || id != wantUser {
			t.Errorf("user id %q err=%v", id, err)
		}
		w.WriteHeader(http.StatusOK)
	})
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHTTPMiddlewareBearerAndCookie(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u1", 1)
	mw := h.HTTPMiddleware()(okHandler(t, "u1"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	if rec := serve(mw, req); rec.Code != http.StatusOK {
		t.Fatalf("bearer: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: corejwt.AccessTokenName, Value: pair.AccessToken})
	if rec := serve(mw, req); rec.Code != http.StatusOK {
		t.Fatalf("cookie: %d", rec.Code)
	}
}

func TestHTTPMiddlewareRejections(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u1", 1)
	mw := h.HTTPMiddleware()(okHandler(t, "u1"))

	cases := map[string]func(*http.Request){
		"none":          func(*http.Request) {},
		"garbage":       func(r *http.Request) { r.Header.Set("Authorization", "Bearer xxx") },
		"refresh token": func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+pair.RefreshToken) },
		"basic scheme":  func(r *http.Request) { r.Header.Set("Authorization", "Basic "+pair.AccessToken) },
	}
	for name, mod := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		mod(req)
		rec := serve(mw, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d", name, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: missing headers %v", name, rec.Header())
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Errorf("%s: bad body %q", name, rec.Body.String())
		}
	}
}

func TestHTTPMiddlewareTokenSources(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u1", 1)
	mw := h.HTTPMiddleware(corejwt.WithTokenSources(corejwt.SourceHeader))(okHandler(t, "u1"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: corejwt.AccessTokenName, Value: pair.AccessToken})
	if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cookie must be ignored, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareRoles(t *testing.T) {
	h := newHelper(t)
	admin, _ := h.GeneratePair("admin", 1)
	user, _ := h.GeneratePair("user", 2)
	mw := h.HTTPMiddleware(corejwt.WithRoles(1))(okHandler(t, "admin"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	if rec := serve(mw, req); rec.Code != http.StatusOK {
		t.Fatalf("admin: %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)
	if rec := serve(mw, req); rec.Code != http.StatusForbidden {
		t.Fatalf("user: want 403, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareCookieRefresh(t *testing.T) {
	h := newHelper(t, corejwt.WithRefreshStore(corejwt.NewMemoryRefreshStore()))
	pair, _ := h.GeneratePair("u1", 1)
	mw := h.HTTPMiddleware(corejwt.WithCookieRefresh())(okHandler(t, "u1"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: corejwt.AccessTokenName, Value: "expired-or-bad"})
	req.AddCookie(&http.Cookie{Name: corejwt.RefreshTokenName, Value: pair.RefreshToken})
	rec := serve(mw, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body)
	}
	if n := len(rec.Result().Cookies()); n != 2 {
		t.Fatalf("expected 2 new cookies, got %d", n)
	}

	// Second use of the same refresh token is rejected (rotation).
	if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse: want 401, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareRefreshCookieCannotBeAccess(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u1", 1)
	mw := h.HTTPMiddleware()(okHandler(t, "u1"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: corejwt.AccessTokenName, Value: pair.RefreshToken})
	if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareCustomErrorHandler(t *testing.T) {
	h := newHelper(t)
	mw := h.HTTPMiddleware(corejwt.WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusTeapot)
	}))(okHandler(t, ""))
	if rec := serve(mw, httptest.NewRequest(http.MethodGet, "/", nil)); rec.Code != http.StatusTeapot {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestDeprecatedMiddleware(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u1", 7)
	called := false
	fn := corejwt.Middleware(func(w http.ResponseWriter, r *http.Request) { called = true }, string(testSecret), 7)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: corejwt.AccessTokenName, Value: pair.AccessToken})
	fn(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("handler not called")
	}
}
