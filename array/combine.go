package array

import "slices"

// Concat returns a new slice with the elements of all slices in order.
// It delegates to slices.Concat.
func Concat[T any](ss ...[]T) []T {
	if r := slices.Concat(ss...); r != nil {
		return r
	}
	return []T{}
}

// Interleave merges slices round-robin: Interleave([1 2], [10]) = [1 10 2].
func Interleave[T any](ss ...[]T) []T {
	total, maxLen := 0, 0
	for _, s := range ss {
		total += len(s)
		maxLen = max(maxLen, len(s))
	}
	result := make([]T, 0, total)
	for i := range maxLen {
		for _, s := range ss {
			if i < len(s) {
				result = append(result, s[i])
			}
		}
	}
	return result
}

// Zip pairs elements of s1 and s2 by index, stopping at the shorter slice.
func Zip[T, U any](s1 []T, s2 []U) []Pair[T, U] {
	n := min(len(s1), len(s2))
	result := make([]Pair[T, U], n)
	for i := range n {
		result[i] = Pair[T, U]{First: s1[i], Second: s2[i]}
	}
	return result
}

// Unzip splits pairs into two slices; it is the inverse of Zip.
func Unzip[T, U any](pairs []Pair[T, U]) ([]T, []U) {
	a, b := make([]T, len(pairs)), make([]U, len(pairs))
	for i, p := range pairs {
		a[i], b[i] = p.First, p.Second
	}
	return a, b
}

// ZipWith combines elements of s1 and s2 by index with fn, stopping at the
// shorter slice.
func ZipWith[T, U, V any](s1 []T, s2 []U, fn func(T, U) V) []V {
	n := min(len(s1), len(s2))
	result := make([]V, n)
	for i := range n {
		result[i] = fn(s1[i], s2[i])
	}
	return result
}

// ZipWithIndex pairs each element with its index.
func ZipWithIndex[T any](s []T) []Pair[int, T] {
	result := make([]Pair[int, T], len(s))
	for i, v := range s {
		result[i] = Pair[int, T]{First: i, Second: v}
	}
	return result
}

// GroupBy groups elements by key, preserving input order inside each group.
func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, v := range s {
		k := key(v)
		result[k] = append(result[k], v)
	}
	return result
}

// KeyBy indexes elements by key; for duplicate keys the last element wins.
func KeyBy[T any, K comparable](s []T, key func(T) K) map[K]T {
	result := make(map[K]T, len(s))
	for _, v := range s {
		result[key(v)] = v
	}
	return result
}

// Partition splits s into the elements for which test is true (pass) and the
// rest (fail), preserving order. Both results live in one allocation and are
// capacity-clipped, so appending to one never affects the other.
func Partition[T any](s []T, test func(T) bool) (pass, fail []T) {
	buf := make([]T, len(s))
	lo, hi := 0, len(s)
	for _, v := range s {
		if test(v) {
			buf[lo] = v
			lo++
		} else {
			hi--
			buf[hi] = v
		}
	}
	pass, fail = buf[:lo:lo], buf[lo:]
	slices.Reverse(fail) // failures were written back to front
	return pass, fail
}

// PartitionBy splits s into runs of consecutive elements with the same key:
// PartitionBy([1 1 2 1], id) = [[1 1] [2] [1]]. Use GroupBy to group
// non-adjacent elements. Groups share memory with s and are capacity-clipped.
func PartitionBy[T any, K comparable](s []T, key func(T) K) [][]T {
	if len(s) == 0 {
		return [][]T{}
	}
	var result [][]T
	start, cur := 0, key(s[0])
	for i := 1; i < len(s); i++ {
		if k := key(s[i]); k != cur {
			result = append(result, s[start:i:i])
			start, cur = i, k
		}
	}
	return append(result, s[start:len(s):len(s)])
}
