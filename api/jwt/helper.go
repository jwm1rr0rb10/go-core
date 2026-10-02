package jwt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jwm1rr0rb10/go-errors"
)

// MinSecretLen is the minimum HS256 secret length accepted by NewHelper.
const MinSecretLen = 32

// Default token lifetimes.
const (
	DefaultAccessTTL  = 5 * time.Minute
	DefaultRefreshTTL = 31 * 24 * time.Hour
)

// Default cookie names.
const (
	AccessTokenName  = "Access-Token"
	RefreshTokenName = "Refresh-Token"
)

// CookieConfig controls the cookies produced by Helper.Cookies.
type CookieConfig struct {
	AccessName  string
	RefreshName string
	Domain      string
	Path        string
	// RefreshPath restricts the refresh cookie to a path (for example the
	// refresh endpoint). Empty means Path.
	RefreshPath string
	SameSite    http.SameSite
	// Secure should only be disabled for local development over plain HTTP.
	Secure bool
}

// DefaultCookieConfig returns Secure, HttpOnly, SameSite=Strict cookies on "/".
func DefaultCookieConfig() CookieConfig {
	return CookieConfig{
		AccessName:  AccessTokenName,
		RefreshName: RefreshTokenName,
		Path:        "/",
		SameSite:    http.SameSiteStrictMode,
		Secure:      true,
	}
}

// Option configures a Helper.
type Option func(*Helper)

// WithAccessTTL sets the access token lifetime. Non-positive values are ignored.
func WithAccessTTL(d time.Duration) Option {
	return func(h *Helper) {
		if d > 0 {
			h.accessTTL = d
		}
	}
}

// WithRefreshTTL sets the refresh token lifetime. Non-positive values are ignored.
func WithRefreshTTL(d time.Duration) Option {
	return func(h *Helper) {
		if d > 0 {
			h.refreshTTL = d
		}
	}
}

// WithIssuer sets the "iss" claim of issued tokens and requires it on parse.
func WithIssuer(iss string) Option {
	return func(h *Helper) { h.issuer = iss }
}

// WithAudience sets the "aud" claim of issued tokens and requires that parsed
// tokens contain at least one of the given values.
func WithAudience(aud ...string) Option {
	return func(h *Helper) { h.audience = append([]string(nil), aud...) }
}

// WithLeeway tolerates clock skew between issuer and verifier.
func WithLeeway(d time.Duration) Option {
	return func(h *Helper) {
		if d >= 0 {
			h.leeway = d
		}
	}
}

// WithClock replaces time.Now, mainly for tests.
func WithClock(now func() time.Time) Option {
	return func(h *Helper) {
		if now != nil {
			h.now = now
		}
	}
}

// WithCookieConfig overrides the cookie settings. Empty names and path fall
// back to the defaults.
func WithCookieConfig(c CookieConfig) Option {
	return func(h *Helper) {
		def := DefaultCookieConfig()
		if c.AccessName == "" {
			c.AccessName = def.AccessName
		}
		if c.RefreshName == "" {
			c.RefreshName = def.RefreshName
		}
		if c.Path == "" {
			c.Path = def.Path
		}
		if c.SameSite == 0 {
			c.SameSite = def.SameSite
		}
		h.cookies = c
	}
}

// WithRefreshStore makes refresh tokens single-use: every Refresh consumes
// the presented token id and a second use fails with ErrTokenRevoked.
func WithRefreshStore(s RefreshStore) Option {
	return func(h *Helper) { h.store = s }
}

// Helper issues and verifies tokens. It is safe for concurrent use.
type Helper struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
	audience   []string
	leeway     time.Duration
	now        func() time.Time
	cookies    CookieConfig
	store      RefreshStore
	parser     *jwt.Parser
}

// NewHelper builds a Helper. The secret must be at least MinSecretLen bytes;
// generate it with a CSPRNG and keep it out of source control.
func NewHelper(secret []byte, opts ...Option) (*Helper, error) {
	if len(secret) < MinSecretLen {
		return nil, ErrWeakSecret
	}
	h := &Helper{
		secret:     append([]byte(nil), secret...),
		accessTTL:  DefaultAccessTTL,
		refreshTTL: DefaultRefreshTTL,
		now:        time.Now,
		cookies:    DefaultCookieConfig(),
	}
	for _, opt := range opts {
		opt(h)
	}

	popts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(h.leeway),
		jwt.WithTimeFunc(h.now),
	}
	if h.issuer != "" {
		popts = append(popts, jwt.WithIssuer(h.issuer))
	}
	if len(h.audience) > 0 {
		popts = append(popts, jwt.WithAudience(h.audience...))
	}
	h.parser = jwt.NewParser(popts...)
	return h, nil
}

// MustNewHelper is like NewHelper but panics on error. Use it only for
// configuration known to be valid at startup.
func MustNewHelper(secret []byte, opts ...Option) *Helper {
	h, err := NewHelper(secret, opts...)
	if err != nil {
		panic(err)
	}
	return h
}

// GeneratePair issues a new access and refresh token for userID.
func (h *Helper) GeneratePair(userID string, roleID uint64) (*Pair, error) {
	if userID == "" {
		return nil, errors.New("jwt: empty user id")
	}
	now := h.now()
	access, accessClaims, err := h.sign(userID, roleID, TypeAccess, now, h.accessTTL)
	if err != nil {
		return nil, err
	}
	refresh, refreshClaims, err := h.sign(userID, roleID, TypeRefresh, now, h.refreshTTL)
	if err != nil {
		return nil, err
	}
	return &Pair{
		AccessToken:   access,
		RefreshToken:  refresh,
		AccessClaims:  accessClaims,
		RefreshClaims: refreshClaims,
	}, nil
}

func (h *Helper) sign(sub string, role uint64, typ TokenType, now time.Time, ttl time.Duration) (string, *Claims, error) {
	jti, err := newID()
	if err != nil {
		return "", nil, err
	}
	c := &Claims{
		RoleID: role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Issuer:    h.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
	}
	if len(h.audience) > 0 {
		c.Audience = jwt.ClaimStrings(h.audience)
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(h.secret)
	if err != nil {
		return "", nil, fmt.Errorf("jwt: sign: %w", err)
	}
	return s, c, nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("jwt: generate id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Parse verifies the signature and standard claims of token and checks that
// it has the expected type. Errors wrap ErrBadToken (and ErrTokenExpired or
// ErrWrongTokenType when applicable).
func (h *Helper) Parse(token string, want TokenType) (*Claims, error) {
	c := &Claims{}
	t, err := h.parser.ParseWithClaims(token, c, h.keyFunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, fmt.Errorf("%w: %w", ErrBadToken, ErrTokenExpired)
		}
		return nil, fmt.Errorf("%w: %w", ErrBadToken, err)
	}
	if !t.Valid {
		return nil, ErrBadToken
	}
	if c.Type != want {
		return nil, fmt.Errorf("%w: %w", ErrBadToken, ErrWrongTokenType)
	}
	if c.Subject == "" || c.ID == "" {
		return nil, fmt.Errorf("%w: missing sub or jti", ErrBadToken)
	}
	return c, nil
}

// ParseAccess is Parse(token, TypeAccess).
func (h *Helper) ParseAccess(token string) (*Claims, error) {
	return h.Parse(token, TypeAccess)
}

// ParseRefresh is Parse(token, TypeRefresh). It does not consult the
// RefreshStore; use Refresh to exchange a refresh token.
func (h *Helper) ParseRefresh(token string) (*Claims, error) {
	return h.Parse(token, TypeRefresh)
}

func (h *Helper) keyFunc(*jwt.Token) (any, error) { return h.secret, nil }

// Refresh validates refreshToken, consumes it in the RefreshStore (when one is
// configured) and issues a new pair for the same subject and role.
func (h *Helper) Refresh(ctx context.Context, refreshToken string) (*Pair, error) {
	c, err := h.ParseRefresh(refreshToken)
	if err != nil {
		return nil, err
	}
	if h.store != nil {
		if err := h.store.Consume(ctx, c.ID, c.ExpiresAt.Time); err != nil {
			return nil, err
		}
	}
	return h.GeneratePair(c.Subject, c.RoleID)
}

// Cookies returns HttpOnly cookies for pair using the configured CookieConfig.
// MaxAge matches the token lifetimes.
func (h *Helper) Cookies(pair *Pair) (access, refresh *http.Cookie) {
	cfg := h.cookies
	refreshPath := cfg.RefreshPath
	if refreshPath == "" {
		refreshPath = cfg.Path
	}
	access = &http.Cookie{
		Name:     cfg.AccessName,
		Value:    pair.AccessToken,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		MaxAge:   int(h.accessTTL / time.Second),
		Secure:   cfg.Secure,
		HttpOnly: true,
		SameSite: cfg.SameSite,
	}
	refresh = &http.Cookie{
		Name:     cfg.RefreshName,
		Value:    pair.RefreshToken,
		Path:     refreshPath,
		Domain:   cfg.Domain,
		MaxAge:   int(h.refreshTTL / time.Second),
		Secure:   cfg.Secure,
		HttpOnly: true,
		SameSite: cfg.SameSite,
	}
	return access, refresh
}

// SetCookies writes both cookies of pair to w.
func (h *Helper) SetCookies(w http.ResponseWriter, pair *Pair) {
	a, r := h.Cookies(pair)
	http.SetCookie(w, a)
	http.SetCookie(w, r)
}

// ClearCookies expires both auth cookies (logout).
func (h *Helper) ClearCookies(w http.ResponseWriter) {
	a, r := h.Cookies(&Pair{})
	a.MaxAge, r.MaxAge = -1, -1
	http.SetCookie(w, a)
	http.SetCookie(w, r)
}

// AccessTTL returns the configured access token lifetime.
func (h *Helper) AccessTTL() time.Duration { return h.accessTTL }

// RefreshTTL returns the configured refresh token lifetime.
func (h *Helper) RefreshTTL() time.Duration { return h.refreshTTL }
