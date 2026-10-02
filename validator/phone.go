package validator

// Phone numbers have 7 to 15 digits (E.164 allows up to 15).
const (
	minPhoneDigits = 7
	maxPhoneDigits = 15
)

type phoneValidator struct {
	field, value string
}

// isValidPhone accepts an optional leading '+' followed by 7–15 digits;
// spaces, dashes and parentheses between digits are ignored. It does not
// allocate.
func isValidPhone(phone string) bool {
	digits := 0
	for i := 0; i < len(phone); i++ {
		switch c := phone[i]; {
		case c >= '0' && c <= '9':
			digits++
		case c == '+':
			if digits > 0 || i != firstNonSeparator(phone) {
				return false
			}
		case c == ' ' || c == '-' || c == '(' || c == ')':
		default:
			return false
		}
	}
	return digits >= minPhoneDigits && digits <= maxPhoneDigits
}

func firstNonSeparator(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '-', '(', ')':
		default:
			return i
		}
	}
	return len(s)
}

func (v phoneValidator) Validate() error {
	if !isValidPhone(v.value) {
		return fieldError(v.field, "must be a valid phone number")
	}
	return nil
}

// PhoneValidator checks for an international phone number: 7–15 digits with
// an optional leading '+', allowing spaces, dashes and parentheses.
func PhoneValidator(field, value string) Validator {
	return phoneValidator{field: field, value: value}
}
