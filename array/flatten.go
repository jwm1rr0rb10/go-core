package array

import (
	"reflect"
	"slices"
)

// Flatten concatenates a slice of slices one level deep. It delegates to
// slices.Concat.
func Flatten[T any](s [][]T) []T { return Concat(s...) }

// maxFlattenDepth bounds recursion in FlattenDeep. Deeper values are kept as
// leaves instead of being expanded.
const maxFlattenDepth = 512

// FlattenDeep recursively flattens nested slices and arrays of any depth and
// element type into a flat []any: FlattenDeep([]any{1, []any{2, []int{3}}})
// = [1 2 3]. Non-slice values (including strings and maps) are leaves; nil
// input yields an empty result.
//
// FlattenDeep is safe on hostile input: a slice that contains itself is
// expanded only once along any path, and nesting deeper than 512 levels is
// kept as a leaf. It uses reflection; prefer Flatten for typed [][]T.
func FlattenDeep(input any) []any {
	result := make([]any, 0, 16)
	if input == nil {
		return result
	}
	var path []uintptr // backing arrays of the slices currently being expanded
	var walk func(v reflect.Value, depth int)
	walk = func(v reflect.Value, depth int) {
		for v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			if depth >= maxFlattenDepth {
				result = append(result, v.Interface())
				return
			}
			if v.Kind() == reflect.Slice && v.Len() > 0 {
				p := v.Pointer()
				if slices.Contains(path, p) {
					return // cycle: the slice contains itself
				}
				path = append(path, p)
				defer func() { path = path[:len(path)-1] }()
			}
			for i := range v.Len() {
				walk(v.Index(i), depth+1)
			}
		case reflect.Invalid:
			result = append(result, nil)
		default:
			result = append(result, v.Interface())
		}
	}
	walk(reflect.ValueOf(input), 0)
	return result
}
