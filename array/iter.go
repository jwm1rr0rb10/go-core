package array

import "iter"

// Lazy counterparts of Map/Filter for iter.Seq pipelines. They allocate
// nothing per element and stop as soon as the consumer stops. Use
// slices.Values / slices.Collect to convert from and to slices, and
// slices.Chunk for a lazy Chunk.

// MapSeq lazily applies transform to each value of seq.
func MapSeq[T, U any](seq iter.Seq[T], transform func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(transform(v)) {
				return
			}
		}
	}
}

// FilterSeq lazily yields the values of seq that satisfy keep.
func FilterSeq[T any](seq iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

// UniqSeq lazily yields the values of seq not seen before.
func UniqSeq[T comparable](seq iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		seen := make(map[T]struct{})
		for v := range seq {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			if !yield(v) {
				return
			}
		}
	}
}

// TakeSeq lazily yields at most n values of seq.
func TakeSeq[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			if i++; i >= n {
				return
			}
		}
	}
}
