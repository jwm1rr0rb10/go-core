package validator

// Validator is anything that can check itself.
type Validator interface {
	Validate() error
}

// ValidatorFunc adapts a function to the Validator interface.
type ValidatorFunc func() error

// Validate calls f.
func (f ValidatorFunc) Validate() error { return f() }

type chainValidator []Validator

func (v chainValidator) Validate() error {
	for _, validator := range v {
		if validator == nil {
			continue
		}
		if err := validator.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ChainValidator runs validators in order and returns the first error.
// Nil validators are skipped.
func ChainValidator(validators ...Validator) Validator {
	return chainValidator(validators)
}

type collectValidator []Validator

func (v collectValidator) Validate() error {
	var merged ErrorFields
	for _, validator := range v {
		if validator == nil {
			continue
		}
		err := validator.Validate()
		if err == nil {
			continue
		}
		vErr, ok := AsValidationError(err)
		if !ok {
			return err // not a validation failure: stop and report it as is
		}
		if merged == nil {
			merged = make(ErrorFields, len(vErr.Fields))
		}
		for field, msg := range vErr.Fields {
			if _, seen := merged[field]; !seen { // first message per field wins
				merged[field] = msg
			}
		}
	}
	if merged == nil {
		return nil
	}
	return ValidationError{Fields: merged}
}

// CollectAll runs every validator and merges all ValidationErrors into one,
// so a client sees every invalid field at once. If a validator returns any
// other error, CollectAll stops and returns that error unchanged. When two
// validators report the same field, the first message is kept.
func CollectAll(validators ...Validator) Validator {
	return collectValidator(validators)
}
