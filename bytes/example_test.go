package bytes_test

import (
	"fmt"

	corebytes "github.com/jwm1rr0rb10/go-core/bytes"
)

func ExampleUnmarshal() {
	type Event struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
	}
	ev, err := corebytes.Unmarshal[Event]([]byte(`{"id":42,"kind":"signup"}`))
	fmt.Println(ev.ID, ev.Kind, err)
	// Output: 42 signup <nil>
}

func ExampleJSONToMap() {
	m, _ := corebytes.JSONToMap([]byte(`{"order_id": 9007199254740993}`), corebytes.UseNumber())
	fmt.Println(m["order_id"])
	// Output: 9007199254740993
}
