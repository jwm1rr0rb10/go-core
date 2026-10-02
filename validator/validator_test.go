package validator_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	playground "github.com/go-playground/validator/v10"

	"github.com/jwm1rr0rb10/go-core/validator"
)

type sample struct {
	Name string `validate:"required"`
	Date string `validate:"date"`
}

type address struct {
	City string `json:"city" validate:"required"`
}

type signup struct {
	Email   string  `json:"email" validate:"required,email"`
	Age     int     `json:"age" validate:"gte=18"`
	Secret  string  `json:"-" validate:"required"`
	Address address `json:"address"`
}

func mustEngine(t testing.TB, opts ...validator.Option) *validator.Engine {
	t.Helper()
	e, err := validator.NewEngine(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestStructValidatorWorksWithoutNew(t *testing.T) {
	// Previously this panicked when New had not been called.
	err := validator.StructValidator(sample{Name: "a", Date: "2026-07-11"}).Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStructValidatorRequiredField(t *testing.T) {
	if err := validator.New("2006-01-02"); err != nil {
		t.Fatal(err)
	}
	err := validator.StructValidator(sample{}).Validate()
	var vErr validator.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
	if _, ok := vErr.Fields["Name"]; !ok {
		t.Fatalf("fields = %v", vErr.Fields)
	}
}

func TestNewReplacesDateLayout(t *testing.T) {
	t.Cleanup(func() { _ = validator.New(validator.DefaultDateLayout) })

	if err := validator.New("02.01.2006"); err != nil {
		t.Fatal(err)
	}
	if err := validator.StructValidator(sample{Name: "a", Date: "11.07.2026"}).Validate(); err != nil {
		t.Fatalf("custom layout rejected: %v", err)
	}
	if err := validator.StructValidator(sample{Name: "a", Date: "2026-07-11"}).Validate(); err == nil {
		t.Fatal("old layout must be rejected after New")
	}
}

func TestEngineDateTag(t *testing.T) {
	e := mustEngine(t)
	if err := e.Struct(sample{Name: "a"}); err != nil {
		t.Fatalf("empty date must pass: %v", err)
	}
	err := e.Struct(sample{Name: "a", Date: "11/07/2026"})
	vErr, ok := validator.AsValidationError(err)
	if !ok || !strings.Contains(vErr.Fields["Date"], "'date'") {
		t.Fatalf("got %v", err)
	}
}

func TestEngineNestedAndJSONNames(t *testing.T) {
	in := signup{Email: "bad", Age: 10}

	goNames := mustEngine(t).Struct(in)
	vErr, _ := validator.AsValidationError(goNames)
	for _, f := range []string{"Email", "Age", "Secret", "Address.City"} {
		if _, ok := vErr.Fields[f]; !ok {
			t.Fatalf("missing %s in %v", f, vErr.Fields)
		}
	}

	jsonNames := mustEngine(t, validator.WithJSONFieldNames()).Struct(in)
	vErr, _ = validator.AsValidationError(jsonNames)
	for _, f := range []string{"email", "age", "Secret", "address.city"} {
		if _, ok := vErr.Fields[f]; !ok {
			t.Fatalf("missing %s in %v", f, vErr.Fields)
		}
	}
	if !strings.Contains(vErr.Fields["age"], "(18)") {
		t.Fatalf("param missing from message: %q", vErr.Fields["age"])
	}
}

func TestEngineInvalidInput(t *testing.T) {
	err := mustEngine(t).Struct(42)
	if _, ok := validator.AsValidationError(err); ok || err == nil {
		t.Fatalf("non-struct must return a non-validation error, got %v", err)
	}
	var inv *playground.InvalidValidationError
	if !errors.As(err, &inv) {
		t.Fatalf("want InvalidValidationError, got %T", err)
	}
}

func TestEngineCustomValidation(t *testing.T) {
	e := mustEngine(t, validator.WithValidation("even", func(fl playground.FieldLevel) bool {
		return fl.Field().Int()%2 == 0
	}))
	type s struct {
		N int `validate:"even"`
	}
	if err := e.Struct(s{N: 2}); err != nil {
		t.Fatal(err)
	}
	if err := e.Struct(s{N: 3}); err == nil {
		t.Fatal("odd accepted")
	}
	if _, err := validator.NewEngine(validator.WithValidation("", nil)); err == nil {
		t.Fatal("empty tag must fail registration")
	}
}

func TestEngineVar(t *testing.T) {
	e := mustEngine(t)
	if err := e.Var("email", "a@b.co", "required,email"); err != nil {
		t.Fatal(err)
	}
	vErr, ok := validator.AsValidationError(e.Var("email", "nope", "required,email"))
	if !ok || vErr.Fields["email"] == "" {
		t.Fatalf("got %v", vErr)
	}
}

func TestEngineStructValidator(t *testing.T) {
	e := mustEngine(t)
	if err := e.StructValidator(sample{}).Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetDefaultIgnoresNil(t *testing.T) {
	before := validator.Default()
	validator.SetDefault(nil)
	if validator.Default() != before {
		t.Fatal("nil must be ignored")
	}
}

func TestConcurrentValidation(t *testing.T) {
	e := mustEngine(t)
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			if i%8 == 0 {
				_ = validator.New(validator.DefaultDateLayout)
			}
			_ = validator.StructValidator(sample{Name: "x"}).Validate()
			_ = e.Struct(signup{})
		})
	}
	wg.Wait()
}

func TestValidationErrorFormatting(t *testing.T) {
	err := validator.ValidationError{Fields: validator.ErrorFields{"b": "bad", "a": "missing"}}
	if got := err.Error(); got != "validation failed: a: missing; b: bad" {
		t.Fatalf("got %q", got)
	}
	if got := (validator.ValidationError{}).Error(); got != "validation failed" {
		t.Fatalf("got %q", got)
	}
}

func TestAsValidationError(t *testing.T) {
	v := validator.ValidationError{Fields: validator.ErrorFields{"a": "b"}}
	if _, ok := validator.AsValidationError(fmt.Errorf("wrap: %w", v)); !ok {
		t.Fatal("value form")
	}
	if _, ok := validator.AsValidationError(wrapped{&v}); !ok {
		t.Fatal("pointer form")
	}
	if _, ok := validator.AsValidationError(errors.New("x")); ok {
		t.Fatal("unrelated error")
	}
	if _, ok := validator.AsValidationError(nil); ok {
		t.Fatal("nil")
	}
}

func TestTimeValidator(t *testing.T) {
	tv := validator.NewTimeValidator("created_at", "2026-07-11", "2006-01-02")
	// Value before Validate parses lazily.
	if tv.Value().Year() != 2026 {
		t.Fatalf("lazy Value = %v", tv.Value())
	}
	if err := tv.Validate(); err != nil {
		t.Fatal(err)
	}

	bad := validator.NewTimeValidator("created_at", "bad", "2006-01-02")
	if _, ok := bad.Parsed(); ok {
		t.Fatal("bad value parsed")
	}
	if !bad.Value().IsZero() {
		t.Fatal("bad value must give zero time")
	}
	vErr, ok := validator.AsValidationError(bad.Validate())
	if !ok || vErr.Fields["created_at"] == "" {
		t.Fatalf("got %v", vErr)
	}
}

func TestUUIDValidator(t *testing.T) {
	valid := "9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6d"
	if err := validator.UUIDValidator("id", valid).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"not-a-uuid", "",
		"9b2f6a8e8b1e4b8a9d9e2f1c3a4b5c6d",
		"{9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6d}",
		"urn:uuid:9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6d",
		"9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6z",
	} {
		if err := validator.UUIDValidator("id", bad).Validate(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEmailValidator(t *testing.T) {
	for _, good := range []string{"alice@example.com", "a.b+tag@sub.example.org", "x@y"} {
		if err := validator.EmailValidator("email", good).Validate(); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
	for _, bad := range []string{
		"", "not-an-email", "Alice <alice@example.com>", "<alice@example.com>",
		" alice@example.com", "alice@example.com ", "alice@", "@example.com",
		"alice@example.com (comment)", strings.Repeat("a", 250) + "@b.co",
	} {
		if err := validator.EmailValidator("email", bad).Validate(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPhoneValidator(t *testing.T) {
	for _, good := range []string{"+1 (555) 123-4567", "+79991234567", "1234567", "(+44) 20 7946 0958"} {
		if err := validator.PhoneValidator("phone", good).Validate(); err != nil {
			t.Errorf("%q rejected", good)
		}
	}
	for _, bad := range []string{"", "123", "1234567890123456", "+1+2345678", "12345+678", "555-CALL-NOW", "++1234567"} {
		if err := validator.PhoneValidator("phone", bad).Validate(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestChainValidatorStopsAtFirst(t *testing.T) {
	calls := 0
	count := validator.ValidatorFunc(func() error { calls++; return nil })
	err := validator.ChainValidator(
		nil,
		validator.EmailValidator("email", "bad"),
		count,
	).Validate()
	if err == nil || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if err := validator.ChainValidator(count, nil).Validate(); err != nil || calls != 1 {
		t.Fatal("chain should pass")
	}
}

func TestCollectAll(t *testing.T) {
	err := validator.CollectAll(
		validator.EmailValidator("email", "bad"),
		nil,
		validator.PhoneValidator("phone", "1"),
		validator.EmailValidator("email", "also bad"), // duplicate field: first wins
		validator.UUIDValidator("id", "9b2f6a8e-8b1e-4b8a-9d9e-2f1c3a4b5c6d"),
	).Validate()
	vErr, ok := validator.AsValidationError(err)
	if !ok || len(vErr.Fields) != 2 {
		t.Fatalf("got %v", err)
	}

	internal := errors.New("db down")
	err = validator.CollectAll(
		validator.EmailValidator("email", "bad"),
		validator.ValidatorFunc(func() error { return internal }),
	).Validate()
	if !errors.Is(err, internal) {
		t.Fatalf("non-validation error must pass through, got %v", err)
	}

	if err := validator.CollectAll(validator.EmailValidator("email", "a@b.c")).Validate(); err != nil {
		t.Fatal(err)
	}
}

// wrapped carries a *ValidationError, which fmt.Errorf("%w") refuses.
type wrapped struct{ err error }

func (w wrapped) Error() string { return w.err.Error() }
func (w wrapped) Unwrap() error { return w.err }
