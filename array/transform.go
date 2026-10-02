package array

// Map returns a new slice with transform applied to each element.
func Map[T, U any](s []T, transform func(T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = transform(v)
	}
	return result
}

// MapIndex is Map with the element index passed to transform.
func MapIndex[T, U any](s []T, transform func(int, T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = transform(i, v)
	}
	return result
}

// FilterMap applies fn to each element and keeps the results for which fn
// returns true, in one pass.
func FilterMap[T, U any](s []T, fn func(T) (U, bool)) []U {
	result := make([]U, 0, len(s))
	for _, v := range s {
		if u, ok := fn(v); ok {
			result = append(result, u)
		}
	}
	return result
}

// FlatMap applies fn to each element and concatenates the results.
func FlatMap[T, U any](s []T, fn func(T) []U) []U {
	result := make([]U, 0, len(s))
	for _, v := range s {
		result = append(result, fn(v)...)
	}
	return result
}

// Filter returns a new slice with the elements that satisfy keep. Unlike
// slices.DeleteFunc it does not modify s.
func Filter[T any](s []T, keep func(T) bool) []T {
	result := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			result = append(result, v)
		}
	}
	return result
}

// Uniq returns the distinct elements of s in order of first appearance.
// slices.Compact only removes adjacent duplicates.
func Uniq[T comparable](s []T) []T {
	return UniqBy(s, func(v T) T { return v })
}

// UniqBy returns the elements of s whose key has not been seen before, in
// order of first appearance.
func UniqBy[T any, K comparable](s []T, key func(T) K) []T {
	result := make([]T, 0, len(s))
	if len(s) < 16 {
		// Small inputs: a linear scan beats allocating a map.
		keys := make([]K, 0, len(s))
	outer:
		for _, v := range s {
			k := key(v)
			for _, seen := range keys {
				if seen == k {
					continue outer
				}
			}
			keys = append(keys, k)
			result = append(result, v)
		}
		return result
	}
	seen := make(map[K]struct{}, len(s))
	for _, v := range s {
		k := key(v)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Compact returns a new slice without zero values ("" , 0, nil, ...).
// Note: slices.Compact has a different meaning (it removes adjacent
// duplicates).
func Compact[T comparable](s []T) []T {
	var zero T
	return CompactBy(s, func(v T) bool { return v == zero })
}

// CompactBy returns a new slice without the elements for which remove is
// true. It is the complement of Filter.
func CompactBy[T any](s []T, remove func(T) bool) []T {
	result := make([]T, 0, len(s))
	for _, v := range s {
		if !remove(v) {
			result = append(result, v)
		}
	}
	return result
}

// Reduce folds s from left to right: reducer(...reducer(initial, s[0])..., s[n-1]).
func Reduce[T, U any](s []T, reducer func(U, T) U, initial U) U {
	acc := initial
	for _, v := range s {
		acc = reducer(acc, v)
	}
	return acc
}

// Number is the set of types Sum accepts.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// Sum returns the sum of s (0 for an empty slice).
func Sum[T Number](s []T) T {
	var total T
	for _, v := range s {
		total += v
	}
	return total
}

// SumBy returns the sum of value(v) over s.
func SumBy[T any, N Number](s []T, value func(T) N) N {
	var total N
	for _, v := range s {
		total += value(v)
	}
	return total
}
