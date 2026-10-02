package array_test

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/jwm1rr0rb10/go-core/array"
)

func ExampleMap() {
	fmt.Println(array.Map([]int{1, 2, 3}, func(v int) string { return fmt.Sprint(v * v) }))
	// Output: [1 4 9]
}

func ExampleChunk() {
	fmt.Println(array.Chunk([]int{1, 2, 3, 4, 5}, 2))
	// Output: [[1 2] [3 4] [5]]
}

func ExamplePartition() {
	even, odd := array.Partition([]int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 })
	fmt.Println(even, odd)
	// Output: [2 4] [1 3 5]
}

func ExampleSortBy() {
	type user struct {
		Name string
		Age  int
	}
	users := []user{{"ann", 31}, {"bob", 25}, {"cid", 31}}
	fmt.Println(array.SortBy(users, func(u user) int { return u.Age }))
	// Output: [{bob 25} {ann 31} {cid 31}]
}

func ExampleSort() {
	fmt.Println(array.Sort([]string{"b", "c", "a"}, cmp.Compare[string]))
	// Output: [a b c]
}

func ExampleJoin() {
	fmt.Println(array.Join([]any{1, "two", 3.5, true}, ", "))
	// Output: 1, two, 3.5, true
}

func ExampleMapSeq() {
	evens := array.FilterSeq(slices.Values([]int{1, 2, 3, 4, 5, 6}), func(v int) bool { return v%2 == 0 })
	fmt.Println(slices.Collect(array.MapSeq(evens, func(v int) int { return v * 10 })))
	// Output: [20 40 60]
}

func ExampleGroupBy() {
	byLen := array.GroupBy([]string{"go", "rust", "c", "java"}, func(s string) int { return len(s) })
	fmt.Println(byLen[4], byLen[1])
	// Output: [rust java] [c]
}
