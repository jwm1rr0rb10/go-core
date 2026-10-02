package validator

import (
	"net/mail"
	"strings"
)

// maxEmailLength is the practical limit from RFC 5321 (path of 256 octets
// minus the angle brackets).
const maxEmailLength = 254

type emailValidator struct {
	field, value string
}

// isValidEmail accepts a bare addr-spec ("user@example.com"). Display-name
// forms ("Alice <a@b.c>"), surrounding spaces and comments, which
// net/mail.ParseAddress tolerates, are rejected.
func isValidEmail(email string) bool {
	if email == "" || len(email) > maxEmailLength || strings.ContainsAny(email, " \t\r\n<>") {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return false
	}
	at := strings.LastIndexByte(email, '@')
	return at > 0 && at < len(email)-1
}

func (v emailValidator) Validate() error {
	if !isValidEmail(v.value) {
		return fieldError(v.field, "must be a valid email address")
	}
	return nil
}

// EmailValidator checks that value is a plain e-mail address (no display
// name, at most 254 characters).
func EmailValidator(field, value string) Validator {
	return emailValidator{field: field, value: value}
}
