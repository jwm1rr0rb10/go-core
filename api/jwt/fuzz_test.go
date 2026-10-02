package jwt_test

import (
	"testing"

	corejwt "github.com/jwm1rr0rb10/go-core/api/jwt"
)

func FuzzParse(f *testing.F) {
	h := corejwt.MustNewHelper(testSecret)
	pair, _ := h.GeneratePair("seed", 1)
	f.Add(pair.AccessToken)
	f.Add(pair.RefreshToken)
	f.Add("")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJub25lIn0.e30.")

	f.Fuzz(func(t *testing.T, tok string) {
		c, err := h.ParseAccess(tok)
		if err == nil && (c == nil || c.Type != corejwt.TypeAccess || c.Subject == "") {
			t.Fatalf("accepted invalid claims %+v", c)
		}
		_, _ = h.ParseRefresh(tok)
	})
}
