// Package array provides generic helpers for querying, transforming,
// combining and ordering slices.
//
// Conventions:
//
//   - Functions never modify their input. Results are new slices, except
//     Chunk, Split, Window and PartitionBy, which return sub-slices sharing
//     memory with the input to avoid copying. Those sub-slices are
//     capacity-clipped, so appending to them never overwrites the input, but
//     writing to their elements does.
//   - Where the standard library already has an equivalent (package slices,
//     maps), the helper delegates to it and says so; prefer the stdlib
//     function in new code when its semantics fit.
//   - Iterator variants (MapSeq, FilterSeq, ...) work with iter.Seq for lazy,
//     allocation-free pipelines.
package array

// Pair holds two values of possibly different types.
type Pair[T, U any] struct {
	First  T
	Second U
}
