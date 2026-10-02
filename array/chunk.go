package array

import "github.com/jwm1rr0rb10/go-errors"

// ErrInvalidSize is returned when a chunk or window size is not positive.
var ErrInvalidSize = errors.New("array: size must be positive")

// Chunk splits s into consecutive chunks of size elements; the last chunk may
// be shorter. It returns an empty result when size <= 0 or s is empty.
//
// Chunks share memory with s (no element copying) but are capacity-clipped,
// so appending to a chunk never overwrites the next one. Use slices.Chunk for
// a lazy iterator.
func Chunk[T any](s []T, size int) [][]T {
	if size <= 0 || len(s) == 0 {
		return [][]T{}
	}
	result := make([][]T, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := min(i+size, len(s))
		result = append(result, s[i:end:end])
	}
	return result
}

// Split is like Chunk but reports ErrInvalidSize when size <= 0.
func Split[T any](s []T, size int) ([][]T, error) {
	if size <= 0 {
		return nil, ErrInvalidSize
	}
	return Chunk(s, size), nil
}

// Window returns every overlapping sub-slice of length size (a sliding
// window): Window([1 2 3], 2) = [[1 2] [2 3]]. It returns an empty result if
// size <= 0 or size > len(s). Windows share memory with s and are
// capacity-clipped.
func Window[T any](s []T, size int) [][]T {
	if size <= 0 || size > len(s) {
		return [][]T{}
	}
	result := make([][]T, len(s)-size+1)
	for i := range result {
		result[i] = s[i : i+size : i+size]
	}
	return result
}
