package jwt

import (
	"context"
	"strings"

	"github.com/jwm1rr0rb10/go-errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// gRPC metadata keys used by the interceptor.
const (
	MetadataAuthorization = "authorization"
	MetadataRefreshToken  = "refresh-token"
)

// UnlistedPolicy decides what happens to methods that are neither public nor
// present in the method→roles map.
type UnlistedPolicy uint8

const (
	// UnlistedAuthenticated requires a valid access token with any role
	// (default).
	UnlistedAuthenticated UnlistedPolicy = iota
	// UnlistedDeny rejects the call with PermissionDenied.
	UnlistedDeny
	// UnlistedPublic lets the call through without authentication (the
	// pre-1.4 behavior).
	UnlistedPublic
)

// GRPCOption configures an AuthInterceptor.
type GRPCOption func(*AuthInterceptor)

// WithMethodRoles maps full method names ("/pkg.Service/Method") to the role
// ids allowed to call them. An empty slice means any authenticated subject.
func WithMethodRoles(m map[string][]uint64) GRPCOption {
	return func(i *AuthInterceptor) {
		for k, v := range m {
			i.roles[k] = append([]uint64(nil), v...)
		}
	}
}

// WithPublicMethods marks methods that skip authentication entirely
// (for example health checks or login).
func WithPublicMethods(methods ...string) GRPCOption {
	return func(i *AuthInterceptor) {
		for _, m := range methods {
			i.public[m] = struct{}{}
		}
	}
}

// WithUnlistedPolicy sets the policy for methods not configured explicitly.
func WithUnlistedPolicy(p UnlistedPolicy) GRPCOption {
	return func(i *AuthInterceptor) { i.unlisted = p }
}

// WithMetadataRefresh enables transparent refresh: when the access token is
// missing or invalid and the "refresh-token" metadata holds a valid refresh
// token, a new pair is issued and sent back in response headers
// ("authorization: Bearer <access>", "refresh-token: <refresh>").
func WithMetadataRefresh() GRPCOption {
	return func(i *AuthInterceptor) { i.refresh = true }
}

// AuthInterceptor authenticates gRPC calls with tokens issued by Helper.
// It is safe for concurrent use.
type AuthInterceptor struct {
	helper   *Helper
	roles    map[string][]uint64
	public   map[string]struct{}
	unlisted UnlistedPolicy
	refresh  bool
}

// NewAuthInterceptor builds an interceptor. With no options every method
// requires a valid access token.
func NewAuthInterceptor(h *Helper, opts ...GRPCOption) *AuthInterceptor {
	i := &AuthInterceptor{
		helper: h,
		roles:  make(map[string][]uint64),
		public: make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// Authorize authenticates ctx for fullMethod and returns a context carrying
// the claims. Errors are gRPC status errors: Unauthenticated for missing or
// invalid credentials, PermissionDenied for role failures.
func (i *AuthInterceptor) Authorize(ctx context.Context, fullMethod string) (context.Context, error) {
	if _, ok := i.public[fullMethod]; ok {
		return ctx, nil
	}
	roles, listed := i.roles[fullMethod]
	if !listed {
		switch i.unlisted {
		case UnlistedPublic:
			return ctx, nil
		case UnlistedDeny:
			return nil, status.Error(codes.PermissionDenied, "method not allowed")
		}
	}

	claims, err := i.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if !hasRole(roles, claims.RoleID) {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	return ContextWithClaims(ctx, claims), nil
}

func (i *AuthInterceptor) authenticate(ctx context.Context) (*Claims, error) {
	token, err := tokenFromMD(ctx)
	if err == nil {
		c, perr := i.helper.ParseAccess(token)
		if perr == nil {
			return c, nil
		}
		err = perr
	}
	if !i.refresh {
		return nil, unauthenticated(err)
	}
	vals := metadata.ValueFromIncomingContext(ctx, MetadataRefreshToken)
	if len(vals) == 0 || vals[0] == "" {
		return nil, unauthenticated(err)
	}
	pair, rerr := i.helper.Refresh(ctx, vals[0])
	if rerr != nil {
		return nil, unauthenticated(rerr)
	}
	md := metadata.Pairs(
		MetadataAuthorization, "Bearer "+pair.AccessToken,
		MetadataRefreshToken, pair.RefreshToken,
	)
	if err := grpc.SetHeader(ctx, md); err != nil {
		return nil, status.Error(codes.Internal, "send refreshed tokens")
	}
	return pair.AccessClaims, nil
}

func unauthenticated(err error) error {
	msg := "invalid token"
	switch {
	case errors.Is(err, ErrNoToken):
		msg = "no token"
	case errors.Is(err, ErrTokenExpired):
		msg = "token expired"
	case errors.Is(err, ErrTokenRevoked):
		msg = "token revoked"
	}
	return status.Error(codes.Unauthenticated, msg)
}

func tokenFromMD(ctx context.Context) (string, error) {
	vals := metadata.ValueFromIncomingContext(ctx, MetadataAuthorization)
	if len(vals) == 0 {
		return "", ErrNoToken
	}
	tok := bearerToken(vals[0])
	if tok == "" {
		return "", ErrNoToken
	}
	return tok, nil
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor.
func (i *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := i.Authorize(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor; handlers see
// the authenticated context through ServerStream.Context.
func (i *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := i.Authorize(ss.Context(), info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &authStream{ServerStream: ss, ctx: ctx})
	}
}

type authStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authStream) Context() context.Context { return s.ctx }

// AuthorizeHandler authorizes ctx using the method from the server transport
// stream. It matches the grpc-ecosystem auth.AuthFunc signature.
//
// Deprecated: use UnaryServerInterceptor / StreamServerInterceptor or Authorize.
func (i *AuthInterceptor) AuthorizeHandler(ctx context.Context) (context.Context, error) {
	method, ok := grpc.Method(ctx)
	if !ok || !strings.HasPrefix(method, "/") {
		return nil, status.Error(codes.Internal, "no method in context")
	}
	return i.Authorize(ctx, method)
}
