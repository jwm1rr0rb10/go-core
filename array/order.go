package array

import (
	"cmp"
	"math/rand/v2"
	"slices"

	"github.com/jwm1rr0rb10/go-errors"
)

// ErrEmpty is returned by functions that need at least one element.
var ErrEmpty = errors.New("array: empty slice")

// Sort returns a sorted copy of s ordered by cmp (negative when a < b, as in
// slices.SortFunc). Use cmp.Compare for ordered types:
//
//	array.Sort(users, func(a, b User) int { return cmp.Compare(a.Age, b.Age) })
func Sort[T any](s []T, cmp func(a, b T) int) []T {
	result := clone(s)
	slices.SortFunc(result, cmp)
	return result
}

// SortStable is Sort that keeps the original order of equal elements.
func SortStable[T any](s []T, cmp func(a, b T) int) []T {
	result := clone(s)
	slices.SortStableFunc(result, cmp)
	return result
}

// SortBy returns a copy of s stably sorted in ascending order of key.
func SortBy[T any, K cmp.Ordered](s []T, key func(T) K) []T {
	return SortStable(s, func(a, b T) int { return cmp.Compare(key(a), key(b)) })
}

// SortLess returns a sorted copy of s ordered by a less function, as the
// previous Sort did.
//
// Deprecated: use Sort with a three-way cmp function, which is faster.
func SortLess[T any](s []T, less func(a, b T) bool) []T {
	return Sort(s, func(a, b T) int {
		switch {
		case less(a, b):
			return -1
		case less(b, a):
			return 1
		}
		return 0
	})
}

// Reverse returns a reversed copy of s.
func Reverse[T any](s []T) []T {
	result := clone(s)
	slices.Reverse(result)
	return result
}

// MinMax returns the smallest and largest elements of s in one pass, or
// ErrEmpty. As with slices.Min/Max, a NaN in s makes both results NaN.
func MinMax[T cmp.Ordered](s []T) (minV, maxV T, err error) {
	if len(s) == 0 {
		return minV, maxV, ErrEmpty
	}
	minV, maxV = s[0], s[0]
	for _, v := range s[1:] {
		minV, maxV = min(minV, v), max(maxV, v)
	}
	return minV, maxV, nil
}

// Shuffle returns a randomly permuted copy of s using the goroutine-safe
// math/rand/v2 global generator.
func Shuffle[T any](s []T) []T { return ShuffleWith(s, nil) }

// ShuffleWith is Shuffle with an explicit random source (nil means the global
// generator). Use a seeded source for reproducible results; a *rand.Rand is
// not safe for concurrent use.
func ShuffleWith[T any](s []T, r *rand.Rand) []T {
	result := clone(s)
	swap := func(i, j int) { result[i], result[j] = result[j], result[i] }
	if r == nil {
		rand.Shuffle(len(result), swap)
	} else {
		r.Shuffle(len(result), swap)
	}
	return result
}

// Rotate returns a copy of s rotated left by k positions (negative k rotates
// right): Rotate([1 2 3 4], 1) = [2 3 4 1].
func Rotate[T any](s []T, k int) []T {
	n := len(s)
	if n == 0 {
		return []T{}
	}
	k = ((k % n) + n) % n
	result := make([]T, n)
	copy(result, s[k:])
	copy(result[n-k:], s[:k])
	return result
}
