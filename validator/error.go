package validator

import (
	"slices"
	"strings"

	"github.com/jwm1rr0rb10/go-errors"
)

// ErrorFields maps a field name to its validation message.
type ErrorFields map[string]string

// ValidationError reports which fields failed validation and why.
//
// The package always returns it as a value, so match it with
//
//	var vErr validator.ValidationError
//	if errors.As(err, &vErr) { ... }
//
// or use AsValidationError, which also accepts *ValidationError.
type ValidationError struct {
	Fields ErrorFields
}

// Error lists the failed fields sorted by name, e.g.
// "validation failed: email: must be a valid email address; name: required".
func (e ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var b strings.Builder
	b.WriteString("validation failed: ")
	for i, k := range keys {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(e.Fields[k])
	}
	return b.String()
}

// AsValidationError extracts a ValidationError from err's chain, accepting
// both value and pointer forms.
func AsValidationError(err error) (ValidationError, bool) {
	if v, ok := errors.AsType[ValidationError](err); ok {
		return v, true
	}
	if p, ok := errors.AsType[*ValidationError](err); ok && p != nil {
		return *p, true
	}
	return ValidationError{}, false
}

// fieldError builds a single-field ValidationError.
func fieldError(field, msg string) ValidationError {
	return ValidationError{Fields: ErrorFields{field: msg}}
}
