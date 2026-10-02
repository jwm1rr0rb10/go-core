package jwt_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	corejwt "github.com/jwm1rr0rb10/go-core/api/jwt"
)

type mockTransportStream struct {
	method string
	header metadata.MD
}

func (m *mockTransportStream) Method() string { return m.method }
func (m *mockTransportStream) SetHeader(md metadata.MD) error {
	m.header = metadata.Join(m.header, md)
	return nil
}
func (m *mockTransportStream) SendHeader(md metadata.MD) error { return m.SetHeader(md) }
func (m *mockTransportStream) SetTrailer(metadata.MD) error    { return nil }

func grpcCtx(method string, kv ...string) (context.Context, *mockTransportStream) {
	ts := &mockTransportStream{method: method}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), ts)
	return metadata.NewIncomingContext(ctx, metadata.Pairs(kv...)), ts
}

func code(err error) codes.Code { return status.Code(err) }

const adminMethod = "/test.Service/Admin"

func TestGRPCAuthorize(t *testing.T) {
	h := newHelper(t)
	admin, _ := h.GeneratePair("admin", 1)
	user, _ := h.GeneratePair("user", 2)

	i := corejwt.NewAuthInterceptor(h,
		corejwt.WithMethodRoles(map[string][]uint64{adminMethod: {1}}),
		corejwt.WithPublicMethods("/grpc.health.v1.Health/Check"),
	)

	tests := []struct {
		name   string
		method string
		md     []string
		want   codes.Code
	}{
		{"admin ok", adminMethod, []string{"authorization", "Bearer " + admin.AccessToken}, codes.OK},
		{"lowercase scheme", adminMethod, []string{"authorization", "bearer " + admin.AccessToken}, codes.OK},
		{"wrong role", adminMethod, []string{"authorization", "Bearer " + user.AccessToken}, codes.PermissionDenied},
		{"no token", adminMethod, nil, codes.Unauthenticated},
		{"refresh as access", adminMethod, []string{"authorization", "Bearer " + admin.RefreshToken}, codes.Unauthenticated},
		{"public", "/grpc.health.v1.Health/Check", nil, codes.OK},
		{"unlisted needs auth", "/x.Y/Z", nil, codes.Unauthenticated},
		{"unlisted any role", "/x.Y/Z", []string{"authorization", "Bearer " + user.AccessToken}, codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := grpcCtx(tt.method, tt.md...)
			_, err := i.Authorize(ctx, tt.method)
			if code(err) != tt.want {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
}

func TestGRPCUnlistedPolicies(t *testing.T) {
	h := newHelper(t)
	ctx, _ := grpcCtx("/x.Y/Z")
	if _, err := corejwt.NewAuthInterceptor(h, corejwt.WithUnlistedPolicy(corejwt.UnlistedPublic)).Authorize(ctx, "/x.Y/Z"); err != nil {
		t.Fatalf("public: %v", err)
	}
	if _, err := corejwt.NewAuthInterceptor(h, corejwt.WithUnlistedPolicy(corejwt.UnlistedDeny)).Authorize(ctx, "/x.Y/Z"); code(err) != codes.PermissionDenied {
		t.Fatalf("deny: %v", err)
	}
}

func TestGRPCMetadataRefresh(t *testing.T) {
	h := newHelper(t, corejwt.WithRefreshStore(corejwt.NewMemoryRefreshStore()))
	pair, _ := h.GeneratePair("u", 1)

	noRefresh := corejwt.NewAuthInterceptor(h)
	ctx, _ := grpcCtx(adminMethod, "refresh-token", pair.RefreshToken)
	if _, err := noRefresh.Authorize(ctx, adminMethod); code(err) != codes.Unauthenticated {
		t.Fatalf("refresh must be opt-in: %v", err)
	}

	i := corejwt.NewAuthInterceptor(h, corejwt.WithMetadataRefresh())
	ctx, ts := grpcCtx(adminMethod, "authorization", "Bearer bad", "refresh-token", pair.RefreshToken)
	out, err := i.Authorize(ctx, adminMethod)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if id, _ := corejwt.GetUserID(out); id != "u" {
		t.Fatalf("user %q", id)
	}
	if len(ts.header.Get("authorization")) != 1 || len(ts.header.Get("refresh-token")) != 1 {
		t.Fatalf("new tokens not sent: %v", ts.header)
	}

	// Reuse of the consumed refresh token fails.
	ctx, _ = grpcCtx(adminMethod, "refresh-token", pair.RefreshToken)
	if _, err := i.Authorize(ctx, adminMethod); code(err) != codes.Unauthenticated {
		t.Fatalf("reuse: %v", err)
	}
}

func TestGRPCUnaryInterceptor(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u", 1)
	i := corejwt.NewAuthInterceptor(h)
	ctx, _ := grpcCtx(adminMethod, "authorization", "Bearer "+pair.AccessToken)

	resp, err := i.UnaryServerInterceptor()(ctx, "req", &grpc.UnaryServerInfo{FullMethod: adminMethod},
		func(ctx context.Context, req any) (any, error) {
			id, err := corejwt.GetUserID(ctx)
			return id, err
		})
	if err != nil || resp != "u" {
		t.Fatalf("resp=%v err=%v", resp, err)
	}

	_, err = i.UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: adminMethod},
		func(context.Context, any) (any, error) { t.Fatal("handler called"); return nil, nil })
	if code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

func TestGRPCStreamInterceptor(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u", 1)
	i := corejwt.NewAuthInterceptor(h)
	ctx, _ := grpcCtx(adminMethod, "authorization", "Bearer "+pair.AccessToken)

	var got string
	err := i.StreamServerInterceptor()(nil, &fakeStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: adminMethod},
		func(_ any, ss grpc.ServerStream) error {
			got, _ = corejwt.GetUserID(ss.Context())
			return nil
		})
	if err != nil || got != "u" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestGRPCDeprecatedAuthorizeHandler(t *testing.T) {
	h := newHelper(t)
	pair, _ := h.GeneratePair("u", 1)
	i := corejwt.NewAuthInterceptor(h)
	ctx, _ := grpcCtx(adminMethod, "authorization", "Bearer "+pair.AccessToken)
	if _, err := i.AuthorizeHandler(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := i.AuthorizeHandler(context.Background()); code(err) != codes.Internal {
		t.Fatalf("want Internal without transport stream, got %v", err)
	}
}
