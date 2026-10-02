package random_test

import (
	"fmt"
	"math/rand/v2"
	"net/netip"

	"github.com/jwm1rr0rb10/go-core/random"
)

func ExampleRandInt() {
	n, err := random.RandInt(nil, 1, 7) // a die roll: 1..6
	fmt.Println(err == nil && n >= 1 && n <= 6)
	// Output: true
}

func ExampleRandString_seeded() {
	// A seeded source makes output reproducible, e.g. in tests.
	r := rand.New(rand.NewPCG(42, 42))
	a, _ := random.RandString(r, 8, nil)
	r = rand.New(rand.NewPCG(42, 42))
	b, _ := random.RandString(r, 8, nil)
	fmt.Println(a == b, len(a))
	// Output: true 8
}

func ExampleSecureString() {
	password, _ := random.SecureString(20, nil)
	fmt.Println(len(password))
	// Output: 20
}

func ExamplePick() {
	color, _ := random.Pick(nil, "red", "green", "blue")
	fmt.Println(color == "red" || color == "green" || color == "blue")
	// Output: true
}

func ExampleRandAddrInPrefix() {
	ip, _ := random.RandAddrInPrefix(nil, netip.MustParsePrefix("10.20.0.0/16"))
	fmt.Println(netip.MustParsePrefix("10.20.0.0/16").Contains(ip))
	// Output: true
}
