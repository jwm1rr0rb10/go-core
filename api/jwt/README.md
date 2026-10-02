# jwt

[← go-core](../../README.md) · [Русская версия](READMEru.md)

HS256 access/refresh token pairs with authentication middleware for `net/http`
and gRPC. Tokens use standard registered claims plus a role id and a token type,
and the type is always enforced: a refresh token is never accepted where an
access token is expected, and the other way round.

```go
import "github.com/jwm1rr0rb10/go-core/api/jwt"
```

## Features

- Typed `Claims`: `sub`, `iss`, `aud`, `iat`, `nbf`, `exp`, `jti` + `role_id`, `typ`
- `access` / `refresh` token type check on every parse
- Only HS256 accepted; `none`, HS512 and every other algorithm are rejected
- Secrets shorter than 32 bytes are rejected when the helper is built
- Options: TTLs, issuer, audience, leeway, clock, cookie settings
- Refresh token rotation with reuse detection through a pluggable `RefreshStore`
  (in-memory implementation included)
- HTTP middleware: `Authorization: Bearer` and/or cookie, role filter, optional
  transparent cookie refresh, JSON errors, custom error handler
- gRPC: unary and stream interceptors, method→roles map, public methods,
  default policy for unlisted methods, optional refresh via metadata
- Context stores claims under an unexported typed key (no collisions)
- No dependency on `grpc-ecosystem/go-grpc-middleware`

## API overview

| Identifier | Purpose |
|---|---|
| `NewHelper(secret, ...Option) (*Helper, error)` / `MustNewHelper` | Build a reusable, concurrency-safe helper |
| `WithAccessTTL`, `WithRefreshTTL`, `WithIssuer`, `WithAudience`, `WithLeeway`, `WithClock`, `WithCookieConfig`, `WithRefreshStore` | Helper options |
| `(*Helper).GeneratePair(userID, roleID)` | Issue access + refresh tokens |
| `(*Helper).ParseAccess` / `ParseRefresh` / `Parse(tok, typ)` | Verify a token of the expected type |
| `(*Helper).Refresh(ctx, refresh)` | Exchange (and consume) a refresh token for a new pair |
| `(*Helper).Cookies` / `SetCookies` / `ClearCookies` | HttpOnly cookies for a pair, logout |
| `(*Helper).HTTPMiddleware(...HTTPOption)` | `func(http.Handler) http.Handler` |
| `WithRoles`, `WithTokenSources`, `WithCookieRefresh`, `WithErrorHandler` | HTTP options |
| `NewAuthInterceptor(h, ...GRPCOption)` | gRPC authenticator |
| `UnaryServerInterceptor()` / `StreamServerInterceptor()` / `Authorize(ctx, method)` | gRPC integration |
| `WithMethodRoles`, `WithPublicMethods`, `WithUnlistedPolicy`, `WithMetadataRefresh` | gRPC options |
| `ClaimsFromContext`, `GetUserID`, `GetRoleID`, `ContextWithClaims` | Context accessors |
| `RefreshStore`, `NewMemoryRefreshStore` | Refresh token rotation |
| `ErrBadToken`, `ErrTokenExpired`, `ErrWrongTokenType`, `ErrTokenRevoked`, `ErrNoToken`, `ErrForbidden`, `ErrWeakSecret`, `ErrNoContext` | Errors for `errors.Is` |

## Usage

### Issue and verify

```go
h, err := jwt.NewHelper(secret, // >= 32 random bytes
	jwt.WithIssuer("auth-service"),
	jwt.WithAudience("api"),
	jwt.WithAccessTTL(10*time.Minute),
	jwt.WithRefreshTTL(7*24*time.Hour),
	jwt.WithLeeway(30*time.Second),
	jwt.WithRefreshStore(jwt.NewMemoryRefreshStore()),
)
if err != nil {
	log.Fatal(err)
}

pair, err := h.GeneratePair("user-42", roleAdmin)
claims, err := h.ParseAccess(pair.AccessToken)   // ok
_, err = h.ParseAccess(pair.RefreshToken)         // errors.Is(err, jwt.ErrWrongTokenType)
next, err := h.Refresh(ctx, pair.RefreshToken)    // new pair
_, err = h.Refresh(ctx, pair.RefreshToken)        // errors.Is(err, jwt.ErrTokenRevoked)
```

### HTTP

```go
auth := h.HTTPMiddleware()                                      // any authenticated user
admin := h.HTTPMiddleware(jwt.WithRoles(roleAdmin))             // admins only
web := h.HTTPMiddleware(jwt.WithCookieRefresh(),                // browser: cookies + silent refresh
	jwt.WithTokenSources(jwt.SourceCookie))

mux.Handle("/api/", auth(apiHandler))
mux.Handle("/admin/", admin(adminHandler))
mux.Handle("/app/", web(appHandler))

func apiHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := jwt.GetUserID(r.Context())
	roleID, _ := jwt.GetRoleID(r.Context()) // uint64
	// ...
}

// Login / logout
h.SetCookies(w, pair)
h.ClearCookies(w)
```

Default error responses: `401` with `WWW-Authenticate: Bearer` and
`{"error":"no token"|"token expired"|"unauthorized"}`, or `403` with
`{"error":"forbidden"}`.

### gRPC

```go
authz := jwt.NewAuthInterceptor(h,
	jwt.WithMethodRoles(map[string][]uint64{
		"/billing.Billing/Refund": {roleAdmin},
		"/billing.Billing/List":   {}, // any authenticated user
	}),
	jwt.WithPublicMethods("/auth.Auth/Login", "/grpc.health.v1.Health/Check"),
	jwt.WithUnlistedPolicy(jwt.UnlistedDeny), // default: UnlistedAuthenticated
)

srv := grpc.NewServer(
	grpc.ChainUnaryInterceptor(authz.UnaryServerInterceptor()),
	grpc.ChainStreamInterceptor(authz.StreamServerInterceptor()),
)
```

Clients send `authorization: Bearer <access>`. Missing or invalid tokens give
`codes.Unauthenticated`, role failures give `codes.PermissionDenied`. With
`WithMetadataRefresh()` a client may also send `refresh-token: <refresh>`; the
new pair comes back in response headers `authorization` and `refresh-token`.

## Performance

Measured on Intel Core Ultra 5 225H, Go 1.27.1 (`go test -bench=. -benchmem`):

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| GeneratePair | 10 947 | 5 672 | 79 |
| ParseAccess | 5 828 | 2 268 | 39 |
| ParseAccess (parallel, 14 threads) | 1 340 | 2 287 | 39 |
| HTTPMiddleware (bearer) | 5 886 | 2 637 | 41 |

`Helper` and `AuthInterceptor` are immutable after construction and lock-free
on the hot path; the parser is built once. The only shared lock is inside
`MemoryRefreshStore`, and it is taken only on refresh.

## Security notes

- Use a random secret of at least 32 bytes (`openssl rand -base64 48`) and
  rotate it through your secret manager. HS256 means every verifier can also
  issue tokens; if that is not acceptable, use an asymmetric scheme (RS256/EdDSA
  with JWKS).
- **Configure a `RefreshStore`.** Without one a stolen refresh token stays valid
  for its full lifetime and can be exchanged any number of times.
  `MemoryRefreshStore` only works for a single instance; with replicas,
  implement `Consume` on Redis (`SET jti 1 NX EXAT exp`) or SQL (`INSERT ... ON
  CONFLICT DO NOTHING`).
- Access tokens cannot be revoked before they expire; keep `AccessTTL` short.
- Cookies are `HttpOnly`, `Secure`, `SameSite=Strict` by default. Set
  `CookieConfig.RefreshPath` to send the refresh cookie only to the refresh endpoint.
- `Secure: false` is only for local development over plain HTTP.

## Migration (from 1.3.x)

| Before | Now |
|---|---|
| `NewHelper(secret string) Helper` | `NewHelper(secret []byte, opts...) (*Helper, error)`; secrets < 32 bytes are rejected |
| `GeneratePair(userID, issuerName, roleID)` | `GeneratePair(userID, roleID)`; issuer is set with `WithIssuer` |
| `ParseToken` + `ParseMapClaims` (`jwt.MapClaims`) | `ParseAccess` / `ParseRefresh` returning `*Claims` |
| `CustomClaims{UserID, IssuerName, ExpireAt, IssuedAt, RoleID}` | `Claims{RoleID, Type, jwt.RegisteredClaims}`; user id is `Subject` / `UserID()` |
| Claim names `id`, `iss_at` | Standard `sub`, `iat`, plus `typ`, `jti`, `nbf`. **Tokens issued by 1.3.x are not accepted.** |
| `PrepareCookies(pair)` | `Cookies(pair)` / `SetCookies(w, pair)` |
| `AccessTokenDuration`, `RefreshTokenDuration` (ints) | `DefaultAccessTTL`, `DefaultRefreshTTL` (`time.Duration`) |
| `Middleware(h, secret, roles...)` | `h.HTTPMiddleware(WithRoles(...), WithCookieRefresh())`. The old function still exists (deprecated) and **panics on secrets shorter than 32 bytes** |
| `GetRoleID(ctx) (int, error)` | `GetRoleID(ctx) (uint64, error)` |
| String context keys, `GetUserIdCtxKey`, `GetRoleIdCtxKey`, grpc_ctxtags | Removed; use `ClaimsFromContext` / `GetUserID` / `GetRoleID` |
| `NewAuthInterceptor(helper, roles)` + `AuthorizeHandler` (grpc_auth) | `NewAuthInterceptor(h, WithMethodRoles(roles), ...)` + `UnaryServerInterceptor` / `StreamServerInterceptor`. `AuthorizeHandler` remains (deprecated) |
| Unlisted gRPC methods were public | They now require a valid token; use `WithUnlistedPolicy(UnlistedPublic)` for the old behavior |
| gRPC metadata refresh always on | Opt in with `WithMetadataRefresh()` |
| gRPC errors were all `PermissionDenied` | `Unauthenticated` for credentials, `PermissionDenied` for roles |
| HTTP errors as plain text | JSON body, `WWW-Authenticate` header; customize with `WithErrorHandler` |
