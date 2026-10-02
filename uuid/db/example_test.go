package db_test

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-core/uuid/db"
)

func ExampleNewV7() {
	id, err := db.NewV7()
	if err != nil {
		panic(err)
	}
	// INSERT INTO users (id, ...) VALUES ($1, ...) — id implements driver.Valuer.
	fmt.Println(id.Version(), len(id.String()))
	// Output: 7 36
}
