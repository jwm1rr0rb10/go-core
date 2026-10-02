package network_test

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-core/uuid/network"
)

func ExampleNewV4() {
	requestID := network.NewV4()
	fmt.Println(requestID.Version())
	// Output: 4
}
