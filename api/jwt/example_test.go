package jwt_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"

	corejwt "github.com/jwm1rr0rb10/go-core/api/jwt"
)

func ExampleNewHelper() {
	// Load a random secret of at least 32 bytes from your secret manager.
	secret := []byte("0123456789abcdef0123456789abcdef")

	h, err := corejwt.NewHelper(secret,
		corejwt.WithIssuer("auth-service"),
		corejwt.WithAudience("api"),
		corejwt.WithAccessTTL(10*time.Minute),
		corejwt.WithRefreshStore(corejwt.NewMemoryRefreshStore()),
	)
	if err != nil {
		panic(err)
	}

	pair, _ := h.GeneratePair("user-42", 1)
	claims, err := h.ParseAccess(pair.AccessToken)
	fmt.Println(claims.UserID(), claims.RoleID, claims.Type, err)

	_, err = h.ParseAccess(pair.RefreshToken)
	fmt.Println(err != nil)
	// Output:
	// user-42 1 access <nil>
	// true
}

func ExampleHelper_Refresh() {
	h := corejwt.MustNewHelper([]byte("0123456789abcdef0123456789abcdef"),
		corejwt.WithRefreshStore(corejwt.NewMemoryRefreshStore()))
	pair, _ := h.GeneratePair("user-42", 1)

	_, err := h.Refresh(context.Background(), pair.RefreshToken)
	fmt.Println("first:", err)
	_, err = h.Refresh(context.Background(), pair.RefreshToken)
	fmt.Println("reuse:", err)
	// Output:
	// first: <nil>
	// reuse: jwt: token revoked
}

func ExampleHelper_HTTPMiddleware() {
	h := corejwt.MustNewHelper([]byte("0123456789abcdef0123456789abcdef"))

	admin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := corejwt.GetUserID(r.Context())
		fmt.Fprintln(w, "hello", id)
	})

	mux := http.NewServeMux()
	mux.Handle("/admin", h.HTTPMiddleware(corejwt.WithRoles(1))(admin))
	mux.Handle("/me", h.HTTPMiddleware(corejwt.WithCookieRefresh())(admin))
	_ = mux
}

func ExampleAuthInterceptor() {
	h := corejwt.MustNewHelper([]byte("0123456789abcdef0123456789abcdef"))

	auth := corejwt.NewAuthInterceptor(h,
		corejwt.WithMethodRoles(map[string][]uint64{
			"/billing.Billing/Refund": {1}, // admins only
		}),
		corejwt.WithPublicMethods("/auth.Auth/Login", "/grpc.health.v1.Health/Check"),
	)

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(auth.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(auth.StreamServerInterceptor()),
	)
	_ = srv
}
