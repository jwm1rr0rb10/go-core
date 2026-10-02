package jwt

import "github.com/jwm1rr0rb10/go-errors"

var (
	// ErrBadToken reports a token that is malformed, has an invalid signature
	// or fails claim validation (expired, wrong issuer or audience, ...).
	ErrBadToken = errors.New("jwt: malformed token")

	// ErrTokenExpired reports a token whose exp claim is in the past.
	// It is returned wrapped together with ErrBadToken.
	ErrTokenExpired = errors.New("jwt: token expired")

	// ErrWrongTokenType reports a refresh token used as an access token or
	// the other way round. It is returned wrapped together with ErrBadToken.
	ErrWrongTokenType = errors.New("jwt: wrong token type")

	// ErrTokenRevoked reports a refresh token that was already used or revoked
	// according to the configured RefreshStore.
	ErrTokenRevoked = errors.New("jwt: token revoked")

	// ErrNoToken reports a request without credentials.
	ErrNoToken = errors.New("jwt: no token")

	// ErrForbidden reports an authenticated subject without a permitted role.
	ErrForbidden = errors.New("jwt: forbidden")

	// ErrWeakSecret is returned by NewHelper when the secret is shorter than
	// MinSecretLen bytes.
	ErrWeakSecret = errors.New("jwt: secret must be at least 32 bytes")

	// ErrNoContext is returned by context accessors when the context carries
	// no authenticated claims.
	ErrNoContext = errors.New("jwt: no claims in context")
)
