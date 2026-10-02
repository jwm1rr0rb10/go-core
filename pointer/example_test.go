package pointer_test

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-core/pointer"
)

type UpdateUser struct {
	Name *string
	Age  *int
}

func ExampleToPointerOrNil() {
	// Only non-empty form values become fields to update.
	req := UpdateUser{
		Name: pointer.ToPointerOrNil("Alice"),
		Age:  pointer.ToPointerOrNil(0),
	}
	fmt.Println(*req.Name, req.Age == nil)
	// Output: Alice true
}

func ExampleValueOr() {
	var limit *int
	fmt.Println(pointer.ValueOr(limit, 100))
	// Output: 100
}

func ExampleCoalesce() {
	var fromFlag *string
	fromEnv := pointer.ToPointer("prod")
	fallback := pointer.ToPointer("dev")
	fmt.Println(*pointer.Coalesce(fromFlag, fromEnv, fallback))
	// Output: prod
}

func ExampleEqual() {
	fmt.Println(pointer.Equal(pointer.ToPointer(3), pointer.ToPointer(3)))
	fmt.Println(pointer.Equal(nil, pointer.ToPointer(3)))
	// Output:
	// true
	// false
}
