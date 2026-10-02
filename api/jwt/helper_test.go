package jwt_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	golangjwt "github.com/golang-jwt/jwt/v5"

	corejwt "github.com/jwm1rr0rb10/go-core/api/jwt"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newHelper(t testing.TB, opts ...corejwt.Option) *corejwt.Helper {
	t.Helper()
	h, err := corejwt.NewHelper(testSecret, opts...)
	if err != nil {
		t.Fatalf("new helper: %v", err)
	}
	return h
}

func TestNewHelperRejectsWeakSecret(t *testing.T) {
	if _, err := corejwt.NewHelper([]byte("short")); !errors.Is(err, corejwt.ErrWeakSecret) {
		t.Fatalf("want ErrWeakSecret, got %v", err)
	}
}

func TestMustNewHelperPanicsOnWeakSecret(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	corejwt.MustNewHelper(nil)
}

func TestGeneratePairAndParse(t *testing.T) {
	h := newHelper(t, corejwt.WithIssuer("auth"), corejwt.WithAudience("api"))
	pair, err := h.GeneratePair("user-1", 42)
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.ParseAccess(pair.AccessToken)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if c.UserID() != "user-1" || c.RoleID != 42 || c.Type != corejwt.TypeAccess || c.Issuer != "auth" || c.ID == "" {
		t.Fatalf("unexpected claims: %+v", c)
	}
	if c.ID == pair.RefreshClaims.ID {
		t.Fatal("access and refresh tokens must have distinct jti")
	}
	if _, err := h.ParseRefresh(pair.RefreshToken); err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
}

func TestGeneratePairRejectsEmptyUser(t *testing.T) {
	if _, err := newHelper(t).GeneratePair("", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestTokenTypeIsEnforced(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u", 1)

	if _, err := h.ParseAccess(pair.RefreshToken); !errors.Is(err, corejwt.ErrWrongTokenType) || !errors.Is(err, corejwt.ErrBadToken) {
		t.Fatalf("refresh accepted as access: %v", err)
	}
	if _, err := h.ParseRefresh(pair.AccessToken); !errors.Is(err, corejwt.ErrWrongTokenType) {
		t.Fatalf("access accepted as refresh: %v", err)
	}
	if _, err := h.Refresh(context.Background(), pair.AccessToken); !errors.Is(err, corejwt.ErrWrongTokenType) {
		t.Fatalf("access accepted by Refresh: %v", err)
	}
}

func TestParseRejectsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	h := newHelper(t, corejwt.WithClock(clock), corejwt.WithAccessTTL(time.Minute))
	pair, _ := h.GeneratePair("u", 1)

	now = now.Add(2 * time.Minute)
	_, err := h.ParseAccess(pair.AccessToken)
	if !errors.Is(err, corejwt.ErrTokenExpired) || !errors.Is(err, corejwt.ErrBadToken) {
		t.Fatalf("want expired, got %v", err)
	}

	lenient := newHelper(t, corejwt.WithClock(clock), corejwt.WithLeeway(5*time.Minute))
	if _, err := lenient.ParseAccess(pair.AccessToken); err != nil {
		t.Fatalf("leeway should accept: %v", err)
	}
}

func TestParseRejectsWrongIssuerAudienceAndSecret(t *testing.T) {
	pair, _ := newHelper(t, corejwt.WithIssuer("a"), corejwt.WithAudience("x")).GeneratePair("u", 1)

	cases := map[string]*corejwt.Helper{
		"issuer":   newHelper(t, corejwt.WithIssuer("b")),
		"audience": newHelper(t, corejwt.WithAudience("y")),
		"secret":   corejwt.MustNewHelper([]byte("ffffffffffffffffffffffffffffffff")),
	}
	for name, h := range cases {
		if _, err := h.ParseAccess(pair.AccessToken); !errors.Is(err, corejwt.ErrBadToken) {
			t.Errorf("%s: want ErrBadToken, got %v", name, err)
		}
	}
}

func TestParseRejectsOtherAlgorithms(t *testing.T) {
	h := newHelper(t)
	claims := golangjwt.MapClaims{
		"sub": "u", "jti": "x", "typ": "access",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	none, _ := golangjwt.NewWithClaims(golangjwt.SigningMethodNone, claims).SignedString(golangjwt.UnsafeAllowNoneSignatureType)
	hs512, _ := golangjwt.NewWithClaims(golangjwt.SigningMethodHS512, claims).SignedString(testSecret)
	for _, tok := range []string{none, hs512, "", "a.b.c"} {
		if _, err := h.ParseAccess(tok); !errors.Is(err, corejwt.ErrBadToken) {
			t.Errorf("token %q accepted: %v", tok, err)
		}
	}
}

func TestParseRejectsMissingSubject(t *testing.T) {
	h := newHelper(t)
	tok, _ := golangjwt.NewWithClaims(golangjwt.SigningMethodHS256, golangjwt.MapClaims{
		"jti": "x", "typ": "access",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}).SignedString(testSecret)
	if _, err := h.ParseAccess(tok); !errors.Is(err, corejwt.ErrBadToken) {
		t.Fatalf("want ErrBadToken, got %v", err)
	}
}

func TestRefreshRotationWithStore(t *testing.T) {
	store := corejwt.NewMemoryRefreshStore()
	h := newHelper(t, corejwt.WithRefreshStore(store))
	pair, _ := h.GeneratePair("u", 3)

	next, err := h.Refresh(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if next.AccessClaims.Subject != "u" || next.AccessClaims.RoleID != 3 {
		t.Fatalf("unexpected claims %+v", next.AccessClaims)
	}
	if _, err := h.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, corejwt.ErrTokenRevoked) {
		t.Fatalf("reuse must fail, got %v", err)
	}
	if _, err := h.Refresh(context.Background(), next.RefreshToken); err != nil {
		t.Fatalf("new refresh token must work: %v", err)
	}
}

func TestRefreshConcurrentReuseOnlyOneWins(t *testing.T) {
	h := newHelper(t, corejwt.WithRefreshStore(corejwt.NewMemoryRefreshStore()))
	pair, _ := h.GeneratePair("u", 1)

	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for range 32 {
		wg.Go(func() {
			if _, err := h.Refresh(context.Background(), pair.RefreshToken); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("want exactly one successful refresh, got %d", ok)
	}
}

func TestMemoryRefreshStoreRevokeAndSweep(t *testing.T) {
	s := corejwt.NewMemoryRefreshStore()
	s.Revoke("a", time.Now().Add(-time.Hour))
	s.Revoke("b", time.Now().Add(time.Hour))
	if err := s.Consume(context.Background(), "b", time.Now().Add(time.Hour)); !errors.Is(err, corejwt.ErrTokenRevoked) {
		t.Fatalf("revoked id accepted: %v", err)
	}
	// The first Consume swept the expired record "a".
	if s.Len() != 1 {
		t.Fatalf("expected expired record to be swept, len=%d", s.Len())
	}
}

func TestCookies(t *testing.T) {
	h := newHelper(t, corejwt.WithCookieConfig(corejwt.CookieConfig{
		Domain: "example.com", RefreshPath: "/auth/refresh", Secure: true,
	}))
	pair, _ := h.GeneratePair("u", 1)
	a, r := h.Cookies(pair)
	if !a.HttpOnly || !a.Secure || a.SameSite != http.SameSiteStrictMode || a.Domain != "example.com" || a.Path != "/" {
		t.Fatalf("access cookie: %+v", a)
	}
	if r.Path != "/auth/refresh" || r.Name != corejwt.RefreshTokenName {
		t.Fatalf("refresh cookie: %+v", r)
	}
	if a.MaxAge != int(corejwt.DefaultAccessTTL/time.Second) || r.MaxAge != int(corejwt.DefaultRefreshTTL/time.Second) {
		t.Fatalf("max age: %d %d", a.MaxAge, r.MaxAge)
	}
}

func TestContextAccessors(t *testing.T) {
	ctx := context.Background()
	if _, err := corejwt.GetUserID(ctx); !errors.Is(err, corejwt.ErrNoContext) {
		t.Fatal("expected ErrNoContext")
	}
	if _, err := corejwt.GetRoleID(ctx); !errors.Is(err, corejwt.ErrNoContext) {
		t.Fatal("expected ErrNoContext")
	}
	c := &corejwt.Claims{RoleID: 9}
	c.Subject = "u"
	ctx = corejwt.ContextWithClaims(ctx, c)
	if id, _ := corejwt.GetUserID(ctx); id != "u" {
		t.Fatal(id)
	}
	if r, _ := corejwt.GetRoleID(ctx); r != 9 {
		t.Fatal(r)
	}
	// A plain string key must not collide with the typed key.
	ctx = context.WithValue(context.Background(), "user_id", "evil") //nolint:staticcheck
	if _, ok := corejwt.ClaimsFromContext(ctx); ok {
		t.Fatal("string key must not satisfy ClaimsFromContext")
	}
}
