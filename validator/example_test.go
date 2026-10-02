package validator_test

import (
	"errors"
	"fmt"

	"github.com/jwm1rr0rb10/go-core/validator"
)

func ExampleEngine_Struct() {
	type CreateUser struct {
		Email    string `json:"email" validate:"required,email"`
		Birthday string `json:"birthday" validate:"required,date"`
	}

	engine, _ := validator.NewEngine(validator.WithJSONFieldNames())

	err := engine.Struct(CreateUser{Email: "alice@example.com", Birthday: "1990-13-01"})

	var vErr validator.ValidationError
	if errors.As(err, &vErr) {
		fmt.Println(vErr.Fields["birthday"])
	}
	// Output: field validation for 'birthday' failed on the 'date' tag
}

func ExampleCollectAll() {
	err := validator.CollectAll(
		validator.EmailValidator("email", "alice"),
		validator.PhoneValidator("phone", "12"),
		validator.UUIDValidator("id", "9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6d"),
	).Validate()
	fmt.Println(err)
	// Output: validation failed: email: must be a valid email address; phone: must be a valid phone number
}

func ExampleTimeValidator() {
	tv := validator.NewTimeValidator("starts_at", "2026-07-11", "2006-01-02")
	if err := tv.Validate(); err == nil {
		fmt.Println(tv.Value().Weekday())
	}
	// Output: Saturday
}
