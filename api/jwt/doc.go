// Package jwt issues and verifies HS256 access/refresh token pairs and
// provides authentication middleware for net/http and gRPC servers.
//
// Tokens carry standard registered claims (sub, iss, aud, iat, nbf, exp, jti)
// plus a role id and a token type ("access" or "refresh"). The type is always
// enforced: an access endpoint never accepts a refresh token and the refresh
// flow never accepts an access token.
//
// A [Helper] is immutable after construction and safe for concurrent use;
// build it once at startup with [NewHelper] and share it.
//
// Refresh tokens are long-lived bearer credentials. Without a [RefreshStore]
// a stolen refresh token stays valid until it expires. Configure
// [WithRefreshStore] to make refresh tokens single-use (rotation with reuse
// detection); [MemoryRefreshStore] works for a single instance, use a shared
// store (Redis, SQL) when running several replicas.
package jwt
