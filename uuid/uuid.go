package uuid

import (
	"bytes"
	"time"

	"github.com/jwm1rr0rb10/go-errors"
)

// UUID is a 128-bit universally unique identifier (RFC 9562).
//
// UUID is a comparable value type: use == to compare and it can be used as a
// map key. The zero value is Nil.
type UUID [16]byte

// Variant is the layout of a UUID as defined by its variant bits.
type Variant byte

// UUID variants (RFC 9562, section 4.1).
const (
	VariantNCS       Variant = iota // Reserved, NCS backward compatibility.
	VariantRFC9562                  // The variant used by this package (RFC 4122 / 9562).
	VariantMicrosoft                // Reserved, Microsoft backward compatibility.
	VariantFuture                   // Reserved for future definition.
)

// String returns the variant name.
func (v Variant) String() string {
	switch v {
	case VariantNCS:
		return "NCS"
	case VariantRFC9562:
		return "RFC9562"
	case VariantMicrosoft:
		return "Microsoft"
	default:
		return "Future"
	}
}

var (
	// Nil is the all-zero UUID.
	Nil UUID
	// Max is the all-ones UUID (RFC 9562, section 5.10).
	Max = UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
)

// Errors returned by parsing and decoding functions. Match them with errors.Is.
var (
	ErrInvalidFormat = errors.New("uuid: invalid format")
	ErrInvalidLength = errors.New("uuid: invalid length")
)

// IsZero reports whether u is Nil.
func (u UUID) IsZero() bool { return u == Nil }

// Version returns the version number stored in u (0-15).
func (u UUID) Version() int { return int(u[6] >> 4) }

// Variant returns the variant encoded in u.
func (u UUID) Variant() Variant {
	switch {
	case u[8]&0x80 == 0:
		return VariantNCS
	case u[8]&0xc0 == 0x80:
		return VariantRFC9562
	case u[8]&0xe0 == 0xc0:
		return VariantMicrosoft
	default:
		return VariantFuture
	}
}

// Time returns the creation time embedded in a version 7 UUID with
// millisecond precision. ok is false for other versions.
func (u UUID) Time() (t time.Time, ok bool) {
	if u.Version() != 7 {
		return time.Time{}, false
	}
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 |
		int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms), true
}

// Compare returns -1, 0 or +1 depending on whether u sorts before, equal to
// or after v in byte order. For version 7 this is creation order.
func (u UUID) Compare(v UUID) int { return bytes.Compare(u[:], v[:]) }

// Bytes returns a copy of the 16 raw bytes of u.
func (u UUID) Bytes() []byte { return append([]byte(nil), u[:]...) }

// FromBytes returns the UUID stored in the 16 raw bytes b.
func FromBytes(b []byte) (UUID, error) {
	var u UUID
	if len(b) != len(u) {
		return Nil, ErrInvalidLength
	}
	copy(u[:], b)
	return u, nil
}

// Must returns u or panics if err is non-nil. It simplifies initialization of
// package-level variables: var id = uuid.Must(uuid.Parse("...")).
func Must(u UUID, err error) UUID {
	if err != nil {
		panic(err)
	}
	return u
}
