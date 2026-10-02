package validator

import (
	"time"
)

// TimeValidator checks that RawValue parses with Layout and keeps the parsed
// time. It is not safe for concurrent use.
type TimeValidator struct {
	Field    string
	RawValue string
	Layout   string

	parsed time.Time
	ok     bool
}

// NewTimeValidator returns a TimeValidator for value in the given layout.
func NewTimeValidator(field, value, layout string) *TimeValidator {
	return &TimeValidator{Field: field, RawValue: value, Layout: layout}
}

// Validate parses RawValue and returns a ValidationError if it does not
// match Layout.
func (v *TimeValidator) Validate() error {
	parsed, err := time.Parse(v.Layout, v.RawValue)
	if err != nil {
		v.parsed, v.ok = time.Time{}, false
		return fieldError(v.Field, "wrong the time format")
	}
	v.parsed, v.ok = parsed, true
	return nil
}

// Value returns the parsed time. If Validate has not succeeded yet it parses
// RawValue now and returns the zero time when that fails; use Parsed to tell
// the two cases apart.
func (v *TimeValidator) Value() time.Time {
	t, _ := v.Parsed()
	return t
}

// Parsed returns the parsed time and whether RawValue is valid, validating
// first if needed.
func (v *TimeValidator) Parsed() (time.Time, bool) {
	if !v.ok {
		_ = v.Validate()
	}
	return v.parsed, v.ok
}
