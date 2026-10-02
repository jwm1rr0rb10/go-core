package uuid

import (
	"database/sql/driver"
	"fmt"
)

const hexDigits = "0123456789abcdef"

// textLen is the length of the canonical form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.
const textLen = 36

// String returns the canonical lowercase form, for example
// "0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f".
func (u UUID) String() string {
	var buf [textLen]byte
	encodeHex(buf[:], u)
	return string(buf[:])
}

// URN returns the RFC 9562 URN form "urn:uuid:<canonical>".
func (u UUID) URN() string {
	var buf [9 + textLen]byte
	copy(buf[:], "urn:uuid:")
	encodeHex(buf[9:], u)
	return string(buf[:])
}

func encodeHex(dst []byte, u UUID) {
	j := 0
	for i, b := range u {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			dst[j] = '-'
			j++
		}
		dst[j] = hexDigits[b>>4]
		dst[j+1] = hexDigits[b&0x0f]
		j += 2
	}
}

// AppendText appends the canonical form of u to b (encoding.TextAppender).
func (u UUID) AppendText(b []byte) ([]byte, error) {
	n := len(b)
	b = append(b, make([]byte, textLen)...)
	encodeHex(b[n:], u)
	return b, nil
}

// MarshalText implements encoding.TextMarshaler. JSON encodes a UUID as a
// string through this method.
func (u UUID) MarshalText() ([]byte, error) {
	b := make([]byte, textLen)
	encodeHex(b, u)
	return b, nil
}

// UnmarshalText implements encoding.TextUnmarshaler. It accepts every form
// that Parse accepts.
func (u *UUID) UnmarshalText(b []byte) error {
	v, err := parse(b)
	if err != nil {
		return err
	}
	*u = v
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler (16 raw bytes).
func (u UUID) MarshalBinary() ([]byte, error) { return u.Bytes(), nil }

// UnmarshalBinary implements encoding.BinaryUnmarshaler (16 raw bytes).
func (u *UUID) UnmarshalBinary(b []byte) error {
	v, err := FromBytes(b)
	if err != nil {
		return err
	}
	*u = v
	return nil
}

// Parse decodes s in any of these forms (hex digits are case-insensitive):
//
//	xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
//	{xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx}
//	urn:uuid:xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
//	xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
func Parse(s string) (UUID, error) { return parse(s) }

// MustParse is like Parse but panics on error. Use it for constants only.
func MustParse(s string) UUID { return Must(Parse(s)) }

func parse[T string | []byte](s T) (UUID, error) {
	var u UUID
	switch len(s) {
	case textLen:
	case textLen + 2:
		if s[0] != '{' || s[textLen+1] != '}' {
			return Nil, ErrInvalidFormat
		}
		s = s[1 : textLen+1]
	case textLen + 9:
		if !hasURNPrefix(s) {
			return Nil, ErrInvalidFormat
		}
		s = s[9:]
	case 32:
		for i := range u {
			hi, ok1 := fromHex(s[2*i])
			lo, ok2 := fromHex(s[2*i+1])
			if !ok1 || !ok2 {
				return Nil, ErrInvalidFormat
			}
			u[i] = hi<<4 | lo
		}
		return u, nil
	default:
		return Nil, ErrInvalidLength
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return Nil, ErrInvalidFormat
	}
	j := 0
	for i := range u {
		if j == 8 || j == 13 || j == 18 || j == 23 {
			j++
		}
		hi, ok1 := fromHex(s[j])
		lo, ok2 := fromHex(s[j+1])
		if !ok1 || !ok2 {
			return Nil, ErrInvalidFormat
		}
		u[i] = hi<<4 | lo
		j += 2
	}
	return u, nil
}

func hasURNPrefix[T string | []byte](s T) bool {
	const p = "urn:uuid:"
	for i := 0; i < len(p); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != p[i] {
			return false
		}
	}
	return true
}

func fromHex(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// Scan implements sql.Scanner. It accepts a string or []byte in any form
// Parse accepts, 16 raw bytes (e.g. MySQL BINARY(16)), and nil (Nil).
func (u *UUID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = Nil
		return nil
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		if len(v) == 16 {
			copy(u[:], v)
			return nil
		}
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("uuid: cannot scan %T", src)
	}
}

// Value implements driver.Valuer and stores u in canonical text form.
func (u UUID) Value() (driver.Value, error) { return u.String(), nil }

// NullUUID is a UUID that may be NULL in a database. It mirrors sql.NullString.
type NullUUID struct {
	UUID  UUID
	Valid bool // Valid is true if UUID is not NULL.
}

// Scan implements sql.Scanner.
func (n *NullUUID) Scan(src any) error {
	if src == nil {
		n.UUID, n.Valid = Nil, false
		return nil
	}
	if err := n.UUID.Scan(src); err != nil {
		n.Valid = false
		return err
	}
	n.Valid = true
	return nil
}

// Value implements driver.Valuer.
func (n NullUUID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.UUID.Value()
}
