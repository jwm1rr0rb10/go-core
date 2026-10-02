package google_uuid_test

import (
	"fmt"

	idgen "github.com/jwm1rr0rb10/go-core/uuid/google_uuid"
)

func ExampleIDGenerator() {
	var gen idgen.IDGenerator = idgen.NewULIDGenerator()
	id := gen.GenerateID()
	fmt.Println(len(id))
	// Output: 26
}
