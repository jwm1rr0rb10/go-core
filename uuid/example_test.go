package uuid_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jwm1rr0rb10/go-core/uuid"
)

func ExampleNewV7() {
	a := uuid.NewV7()
	b := uuid.NewV7()
	fmt.Println(a.Version(), a.Compare(b) < 0)
	// Output: 7 true
}

func ExampleParse() {
	u, err := uuid.Parse("{0190A3C2-7B1E-7C3A-9F2D-4B6E8A1C0D3F}")
	if err != nil {
		panic(err)
	}
	ts, _ := u.Time()
	fmt.Println(u)
	fmt.Println(u.Version(), u.Variant())
	fmt.Println(ts.UTC().Format(time.RFC3339))
	// Output:
	// 0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f
	// 7 RFC9562
	// 2024-07-11T21:45:50Z
}

func ExampleUUID_MarshalText() {
	type user struct {
		ID uuid.UUID `json:"id"`
	}
	out, _ := json.Marshal(user{ID: uuid.MustParse("0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f")})
	fmt.Println(string(out))
	// Output: {"id":"0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f"}
}

func ExampleNewGenerator() {
	fixed := time.UnixMilli(1_700_000_000_000)
	gen := uuid.NewGenerator(uuid.WithClock(func() time.Time { return fixed }))
	a, _ := gen.NewV7()
	b, _ := gen.NewV7() // same millisecond: still ordered
	fmt.Println(a.Compare(b))
	// Output: -1
}
