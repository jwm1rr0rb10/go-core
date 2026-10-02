package jwt

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jwm1rr0rb10/go-errors"
)

// TokenSource selects where the HTTP middleware looks for the access token.
type TokenSource uint8

const (
	// SourceHeader reads "Authorization: Bearer <token>".
	SourceHeader TokenSource = 1 << iota
	// SourceCookie reads the access cookie (CookieConfig.AccessName).
	SourceCookie
)

// ErrorHandler writes the response for a failed authentication or
// authorization. err wraps ErrNoToken, ErrBadToken, ErrTokenRevoked or
// ErrForbidden.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// HTTPOption configures the HTTP middleware.
type HTTPOption func(*httpConfig)

type httpConfig struct {
	sources     TokenSource
	roles       []uint64
	autoRefresh bool
	onError     ErrorHandler
}

// WithRoles restricts access to subjects with one of the given role ids.
// Without it any authenticated subject passes.
func WithRoles(roles ...uint64) HTTPOption {
	return func(c *httpConfig) { c.roles = append([]uint64(nil), roles...) }
}

// WithTokenSources selects token sources (default SourceHeader|SourceCookie;
// the header wins when both are present).
func WithTokenSources(s TokenSource) HTTPOption {
	return func(c *httpConfig) {
		if s != 0 {
			c.sources = s
		}
	}
}

// WithCookieRefresh enables transparent refresh: when the access cookie is
// missing or invalid and a valid refresh cookie is present, a new pair is
// issued, written as cookies, and the request proceeds.
func WithCookieRefresh() HTTPOption {
	return func(c *httpConfig) { c.autoRefresh = true }
}

// WithErrorHandler replaces the default JSON error writer.
func WithErrorHandler(fn ErrorHandler) HTTPOption {
	return func(c *httpConfig) {
		if fn != nil {
			c.onError = fn
		}
	}
}

// DefaultErrorHandler writes {"error": "..."} with 401 (plus
// WWW-Authenticate) for authentication failures and 403 for ErrForbidden.
func DefaultErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	status, msg := http.StatusUnauthorized, "unauthorized"
	switch {
	case errors.Is(err, ErrForbidden):
		status, msg = http.StatusForbidden, "forbidden"
	case errors.Is(err, ErrTokenExpired):
		msg = "token expired"
	case errors.Is(err, ErrNoToken):
		msg = "no token"
	}
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{msg})
}

// HTTPMiddleware returns net/http middleware that authenticates requests and
// stores the claims in the request context (see ClaimsFromContext).
func (h *Helper) HTTPMiddleware(opts ...HTTPOption) func(http.Handler) http.Handler {
	cfg := httpConfig{sources: SourceHeader | SourceCookie, onError: DefaultErrorHandler}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := h.authenticateHTTP(w, r, &cfg)
			if err != nil {
				cfg.onError(w, r, err)
				return
			}
			if !hasRole(cfg.roles, claims.RoleID) {
				cfg.onError(w, r, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ContextWithClaims(r.Context(), claims)))
		})
	}
}

func (h *Helper) authenticateHTTP(w http.ResponseWriter, r *http.Request, cfg *httpConfig) (*Claims, error) {
	token := ""
	if cfg.sources&SourceHeader != 0 {
		token = bearerToken(r.Header.Get("Authorization"))
	}
	if token == "" && cfg.sources&SourceCookie != 0 {
		if c, err := r.Cookie(h.cookies.AccessName); err == nil {
			token = c.Value
		}
	}

	var err error
	if token != "" {
		var c *Claims
		if c, err = h.ParseAccess(token); err == nil {
			return c, nil
		}
	} else {
		err = ErrNoToken
	}

	if !cfg.autoRefresh {
		return nil, err
	}
	rc, cerr := r.Cookie(h.cookies.RefreshName)
	if cerr != nil || rc.Value == "" {
		return nil, err
	}
	pair, rerr := h.Refresh(r.Context(), rc.Value)
	if rerr != nil {
		return nil, rerr
	}
	h.SetCookies(w, pair)
	return pair.AccessClaims, nil
}

// bearerToken extracts the token from an "Authorization: Bearer <t>" value.
func bearerToken(v string) string {
	scheme, tok, ok := strings.Cut(v, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(tok)
}

// Middleware is the pre-1.4 convenience wrapper: cookie/header auth with
// transparent cookie refresh and an optional role filter.
//
// It panics if secret is shorter than MinSecretLen bytes.
//
// Deprecated: build a Helper once with NewHelper and use Helper.HTTPMiddleware.
func Middleware(next http.HandlerFunc, secret string, roleID ...uint64) http.HandlerFunc {
	h := MustNewHelper([]byte(secret))
	mw := h.HTTPMiddleware(WithRoles(roleID...), WithCookieRefresh())
	return mw(next).ServeHTTP
}
