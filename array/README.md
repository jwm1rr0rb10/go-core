# array

[← go-core](../README.md) · [Русская версия](READMEru.md)

Generic helpers for querying, transforming, combining and ordering slices, plus lazy
`iter.Seq` variants. Functions never modify their input, delegate to `slices`/`maps` where
the standard library already has the operation, and avoid reflection and needless allocations.

```go
import "github.com/jwm1rr0rb10/go-core/array"
```

## Conventions

- **Inputs are never modified.** Results are new slices, except `Chunk`, `Split`, `Window`
  and `PartitionBy`, which return sub-slices sharing memory with the input (zero copy). Those
  sub-slices are capacity-clipped: `append` on them never overwrites neighbouring data, but
  assigning to their elements writes to the input.
- Empty results are non-nil (`[]T{}`), so they marshal to JSON as `[]`, not `null`.
- Stdlib equivalents are noted below; use them directly in new code when the semantics fit.

## API

| Group | Functions | Stdlib equivalent |
|---|---|---|
| Transform | `Map`, `MapIndex`, `FilterMap`, `FlatMap`, `Filter`, `Reduce`, `Sum`, `SumBy` | — |
| Dedupe / clean | `Uniq`, `UniqBy`, `Compact` (drops zero values), `CompactBy`, `RemoveAll` | `slices.Compact` removes only *adjacent* duplicates |
| Query | `Contains`, `IndexOf`, `Find`, `FindLast`, `Count`, `Every`, `Some`, `None` | `slices.Contains`, `slices.Index`, `slices.IndexFunc`, `slices.ContainsFunc` |
| Compare | `AreIdentical` (multiset equality), `DistinctCount` | `slices.Equal` (ordered) |
| Modify (copy) | `RemoveByValue`, `RemoveByIndex`, `Take`, `Drop`, `DropRight`, `TakeWhile`, `DropWhile`, `Fill` | `slices.Delete` (mutates) |
| Combine | `Concat`, `Flatten`, `Interleave`, `Zip`, `Unzip`, `ZipWith`, `ZipWithIndex` | `slices.Concat` |
| Group | `GroupBy`, `KeyBy`, `Partition`, `PartitionBy` | — |
| Split | `Chunk`, `Split`, `Window` | `slices.Chunk` (iterator) |
| Order (copy) | `Sort`, `SortStable`, `SortBy`, `Reverse`, `Rotate`, `Shuffle`, `ShuffleWith`, `MinMax` | `slices.SortFunc`, `slices.Reverse`, `slices.Min/Max` (mutate / two passes) |
| Strings | `Join` (like `%v`), `JoinString` (`fmt.Stringer`) | `strings.Join` for `[]string` |
| Reflection | `FlattenDeep` (any nesting depth, cycle-safe) | — |
| Iterators | `MapSeq`, `FilterSeq`, `UniqSeq`, `TakeSeq` | `slices.Values`, `slices.Collect` |
| Types | `Pair[T, U]`, `Number` | — |

Errors: `ErrInvalidSize`, `ErrIndexOutOfRange`, `ErrNegativeCount`, `ErrEmpty`.

## Examples

```go
squares := array.Map([]int{1, 2, 3}, func(v int) int { return v * v }) // [1 4 9]

even, odd := array.Partition([]int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 })

byAge := array.SortBy(users, func(u User) int { return u.Age }) // stable
byName := array.Sort(users, func(a, b User) int { return cmp.Compare(a.Name, b.Name) })

for _, batch := range array.Chunk(ids, 500) {
	_ = repo.DeleteBatch(ctx, batch)
}

csv := array.Join([]any{1, "two", 3.5}, ",") // "1,two,3.5"

// Lazy pipeline: no intermediate slices
top := slices.Collect(array.TakeSeq(
	array.FilterSeq(slices.Values(events), func(e Event) bool { return e.Critical }), 10))
```

## Performance

Intel Core Ultra 5 225H, Go 1.27, 10 000 `int`s unless stated:

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| `Map` | 19 400 | 81 920 | 1 |
| `Filter` | 23 900 | 81 920 | 1 |
| `Partition` | 42 100 | 81 920 | 1 (both halves share one buffer) |
| `Uniq` (1 000 distinct) | 194 000 | 377 475 | 34 |
| `Uniq` (10 elements, no map) | 128 | 160 | 2 |
| `Chunk(100)` | 920 | 2 688 | 1 |
| `Join` 1 000 ints | 14 150 (was 57 300) | 9 472 | 2 (was 1 735) |
| `Join` 1 000 strings | 8 190 (was 65 000) | 6 144 | 1 (was 2 001) |

All functions are pure: they are safe to call concurrently as long as the input slice is
not being modified at the same time.

## Migration

- `Sort(s, less func(a, b T) bool)` → `Sort(s, cmp func(a, b T) int)` (faster, matches
  `slices.SortFunc`). Quick fix: `array.Sort(s, cmp.Compare[int])`, or keep the old function
  under the deprecated name `SortLess`.
- `Chunk`, `Split`, `Window` results are now capacity-clipped; appending to a chunk no
  longer overwrites the next chunk / the input.
- `PartitionBy` groups now share memory with the input instead of being copies.
- `Fill` with `end < start` no longer fills up to the end of the slice; it fills nothing.
- `FlattenDeep` no longer hangs on self-referencing slices and caps depth at 512.
- Errors are sentinels; match with `errors.Is`.
- New: `MapIndex`, `FilterMap`, `FlatMap`, `UniqBy`, `RemoveAll`, `FindLast`, `Count`, `None`,
  `Unzip`, `KeyBy`, `SortStable`, `SortBy`, `ShuffleWith`, `Sum`, `SumBy`, `MapSeq`,
  `FilterSeq`, `UniqSeq`, `TakeSeq`.
