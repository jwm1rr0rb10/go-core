package validator_test

import (
	"testing"

	"github.com/jwm1rr0rb10/go-core/validator"
)

func BenchmarkEngineStructValid(b *testing.B) {
	e := mustEngine(b)
	in := signup{Email: "alice@example.com", Age: 30, Secret: "s", Address: address{City: "Minsk"}}
	for b.Loop() {
		if err := e.Struct(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineStructInvalid(b *testing.B) {
	e := mustEngine(b, validator.WithJSONFieldNames())
	in := signup{Email: "bad", Age: 1}
	for b.Loop() {
		if err := e.Struct(in); err == nil {
			b.Fatal("expected error")
		}
	}
}

func BenchmarkEmailValidator(b *testing.B) {
	for b.Loop() {
		_ = validator.EmailValidator("email", "alice@example.com").Validate()
	}
}

func BenchmarkPhoneValidator(b *testing.B) {
	for b.Loop() {
		_ = validator.PhoneValidator("phone", "+1 (555) 123-4567").Validate()
	}
}
