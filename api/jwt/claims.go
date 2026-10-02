package jwt

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType distinguishes access tokens from refresh tokens. It is stored in
// the "typ" claim.
type TokenType string

const (
	// TypeAccess marks short-lived tokens that authorize API calls.
	TypeAccess TokenType = "access"
	// TypeRefresh marks long-lived tokens that can only obtain a new pair.
	TypeRefresh TokenType = "refresh"
)

// Claims is the payload of every token issued by Helper.
//
// The user id is stored in the standard "sub" claim and a unique token id in
// "jti".
type Claims struct {
	RoleID uint64    `json:"role_id"`
	Type   TokenType `json:"typ"`
	jwt.RegisteredClaims
}

// UserID returns the subject of the token.
func (c *Claims) UserID() string { return c.Subject }

// Pair is an access token together with the refresh token issued with it.
type Pair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	// AccessClaims and RefreshClaims describe the issued tokens; they are not
	// serialized.
	AccessClaims  *Claims `json:"-"`
	RefreshClaims *Claims `json:"-"`
}

type claimsKey struct{}

// ContextWithClaims returns a copy of ctx carrying c. Middleware calls it after
// successful authentication; it is exported for tests and custom transports.
func ContextWithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFromContext returns the claims stored by the middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok && c != nil
}

// GetUserID returns the authenticated user id from ctx.
func GetUserID(ctx context.Context) (string, error) {
	c, ok := ClaimsFromContext(ctx)
	if !ok || c.Subject == "" {
		return "", ErrNoContext
	}
	return c.Subject, nil
}

// GetRoleID returns the authenticated role id from ctx.
func GetRoleID(ctx context.Context) (uint64, error) {
	c, ok := ClaimsFromContext(ctx)
	if !ok {
		return 0, ErrNoContext
	}
	return c.RoleID, nil
}

// hasRole reports whether roles is empty (any authenticated subject) or
// contains role.
func hasRole(roles []uint64, role uint64) bool {
	if len(roles) == 0 {
		return true
	}
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
