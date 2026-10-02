package array

import (
	"slices"

	"github.com/jwm1rr0rb10/go-errors"
)

// Errors returned for invalid arguments.
var (
	ErrIndexOutOfRange = errors.New("array: index out of range")
	ErrNegativeCount   = errors.New("array: count must be non-negative")
)

// RemoveByValue returns a copy of s without the first element equal to
// value. If value is absent it returns an unchanged copy.
func RemoveByValue[T comparable](s []T, value T) []T {
	i := slices.Index(s, value)
	if i < 0 {
		return slices.Clone(s)
	}
	return slices.Concat(s[:i], s[i+1:])
}

// RemoveAll returns a copy of s without any element equal to value.
func RemoveAll[T comparable](s []T, value T) []T {
	return CompactBy(s, func(v T) bool { return v == value })
}

// RemoveByIndex returns a copy of s without the element at index, or
// ErrIndexOutOfRange.
func RemoveByIndex[T any](s []T, index int) ([]T, error) {
	if index < 0 || index >= len(s) {
		return nil, ErrIndexOutOfRange
	}
	return slices.Concat(s[:index], s[index+1:]), nil
}

// Take returns a copy of the first n elements (all of s if n >= len(s)), or
// ErrNegativeCount when n < 0.
func Take[T any](s []T, n int) ([]T, error) {
	if n < 0 {
		return nil, ErrNegativeCount
	}
	return clone(s[:min(n, len(s))]), nil
}

// Drop returns a copy of s without its first n elements.
func Drop[T any](s []T, n int) []T {
	return clone(s[min(max(n, 0), len(s)):])
}

// DropRight returns a copy of s without its last n elements.
func DropRight[T any](s []T, n int) []T {
	return clone(s[:len(s)-min(max(n, 0), len(s))])
}

// DropWhile returns a copy of s without the longest prefix whose elements
// satisfy drop.
func DropWhile[T any](s []T, drop func(T) bool) []T {
	i := slices.IndexFunc(s, func(v T) bool { return !drop(v) })
	if i < 0 {
		return []T{}
	}
	return clone(s[i:])
}

// TakeWhile returns a copy of the longest prefix of s whose elements satisfy
// keep.
func TakeWhile[T any](s []T, keep func(T) bool) []T {
	i := slices.IndexFunc(s, func(v T) bool { return !keep(v) })
	if i < 0 {
		return clone(s)
	}
	return clone(s[:i])
}

// Fill returns a copy of s with the elements in [start, end) set to value.
// start and end are clamped to [0, len(s)]; if end <= start nothing is
// filled.
func Fill[T any](s []T, value T, start, end int) []T {
	result := clone(s)
	start = min(max(start, 0), len(result))
	end = min(max(end, 0), len(result))
	if start < end {
		for i := range result[start:end] {
			result[start+i] = value
		}
	}
	return result
}

// clone is slices.Clone that returns an empty, non-nil slice for empty input,
// so results always marshal as [] rather than null.
func clone[T any](s []T) []T {
	if len(s) == 0 {
		return []T{}
	}
	return slices.Clone(s)
}
