package pointer

// ToPointer returns a pointer to a copy of v. It is useful for literals and
// constants, which cannot be addressed directly: ToPointer(42),
// ToPointer("draft"). On Go 1.26+ new(v) is equivalent.
func ToPointer[T any](v T) *T {
	return &v
}

// ToPointerOrNil returns nil if v is the zero value of T and a pointer to a
// copy of v otherwise. It maps "unset" values to absent optional fields,
// e.g. for PATCH requests or nullable database columns.
func ToPointerOrNil[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// FromPointer dereferences p. It returns the zero value and false if p is nil.
func FromPointer[T any](p *T) (value T, ok bool) {
	if p == nil {
		return value, false
	}
	return *p, true
}

// Deref returns *p, or the zero value of T if p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// ValueOr returns *p, or fallback if p is nil.
func ValueOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// FromPointerOr returns *p, or fallback if p is nil. It is the same as ValueOr.
func FromPointerOr[T any](p *T, fallback T) T {
	return ValueOr(p, fallback)
}

// Coalesce returns the first non-nil pointer, or nil if all are nil.
func Coalesce[T any](ps ...*T) *T {
	for _, p := range ps {
		if p != nil {
			return p
		}
	}
	return nil
}

// Equal reports whether a and b are both nil, or both non-nil and point to
// equal values.
func Equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// IsNil reports whether p is nil.
//
// Deprecated: compare with nil directly (p == nil).
func IsNil[T any](p *T) bool {
	return p == nil
}

// Swap exchanges the values that a and b point to.
// It panics if either pointer is nil.
func Swap[T any](a, b *T) {
	if a == nil || b == nil {
		panic("pointer: nil pointer in swap operation")
	}
	*a, *b = *b, *a
}

// Clone returns a pointer to a new copy of *p, or nil if p is nil.
// The copy is shallow: slices, maps and pointers inside the value are shared
// with the original.
func Clone[T any](p *T) *T {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// Copy returns a pointer to a new shallow copy of *p, or nil if p is nil.
// The error is always nil; it remains for compatibility.
//
// Deprecated: use Clone. Copy no longer rejects non-comparable types.
func Copy[T any](p *T) (*T, error) {
	return Clone(p), nil
}

// Set assigns v to *p if p is non-nil and reports whether it did.
func Set[T any](p *T, v T) bool {
	if p == nil {
		return false
	}
	*p = v
	return true
}
