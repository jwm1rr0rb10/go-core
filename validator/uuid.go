package validator

import "github.com/google/uuid"

type uuidValidator struct {
	field, value string
}

// isValidUUID accepts only the canonical 36-character form
// (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx). uuid.Parse alone would also accept
// "urn:uuid:" prefixes, braces and the 32-digit form without dashes.
func isValidUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	_, err := uuid.Parse(id)
	return err == nil
}

func (v uuidValidator) Validate() error {
	if !isValidUUID(v.value) {
		return fieldError(v.field, "must be uuid")
	}
	return nil
}

// UUIDValidator checks that value is a UUID in canonical text form.
func UUIDValidator(field, value string) Validator {
	return uuidValidator{field: field, value: value}
}
