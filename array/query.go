package array

import "slices"

// Contains reports whether e is in s. It delegates to slices.Contains.
func Contains[T comparable](s []T, e T) bool { return slices.Contains(s, e) }

// IndexOf returns the index of the first value in s, or -1. It delegates to
// slices.Index.
func IndexOf[T comparable](s []T, value T) int { return slices.Index(s, value) }

// Find returns the first element satisfying match and true, or the zero value
// and false.
func Find[T any](s []T, match func(T) bool) (T, bool) {
	if i := slices.IndexFunc(s, match); i >= 0 {
		return s[i], true
	}
	var zero T
	return zero, false
}

// FindLast returns the last element satisfying match and true, or the zero
// value and false.
func FindLast[T any](s []T, match func(T) bool) (T, bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if match(s[i]) {
			return s[i], true
		}
	}
	var zero T
	return zero, false
}

// AreIdentical reports whether x and y hold the same elements with the same
// multiplicities, in any order. Use slices.Equal for order-sensitive equality.
func AreIdentical[T comparable](x, y []T) bool {
	if len(x) != len(y) {
		return false
	}
	counts := make(map[T]int, len(x))
	for _, v := range x {
		counts[v]++
	}
	for _, v := range y {
		if counts[v] == 0 {
			return false
		}
		counts[v]--
	}
	return true
}

// DistinctCount returns how many times each distinct element occurs in s.
func DistinctCount[T comparable](s []T) map[T]int {
	counts := make(map[T]int)
	for _, v := range s {
		counts[v]++
	}
	return counts
}

// Count returns the number of elements satisfying test.
func Count[T any](s []T, test func(T) bool) int {
	n := 0
	for _, v := range s {
		if test(v) {
			n++
		}
	}
	return n
}

// Every reports whether all elements satisfy test (true for an empty slice).
func Every[T any](s []T, test func(T) bool) bool {
	return !slices.ContainsFunc(s, func(v T) bool { return !test(v) })
}

// Some reports whether any element satisfies test. It delegates to
// slices.ContainsFunc.
func Some[T any](s []T, test func(T) bool) bool { return slices.ContainsFunc(s, test) }

// None reports whether no element satisfies test.
func None[T any](s []T, test func(T) bool) bool { return !slices.ContainsFunc(s, test) }
