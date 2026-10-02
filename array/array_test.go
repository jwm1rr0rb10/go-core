package array_test

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/jwm1rr0rb10/go-core/array"
)

func eq[T any](t *testing.T, name string, got, want T) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Fatalf("%s = %s, want %s", name, g, w)
	}
}

func TestTransform(t *testing.T) {
	in := []int{1, 2, 2, 3, 4}
	eq(t, "Map", array.Map(in, func(v int) int { return v * 2 }), []int{2, 4, 4, 6, 8})
	eq(t, "MapIndex", array.MapIndex([]string{"a", "b"}, func(i int, s string) string { return fmt.Sprint(i, s) }), []string{"0a", "1b"})
	eq(t, "Filter", array.Filter(in, func(v int) bool { return v%2 == 0 }), []int{2, 2, 4})
	eq(t, "FilterMap", array.FilterMap(in, func(v int) (string, bool) { return fmt.Sprint(v), v > 2 }), []string{"3", "4"})
	eq(t, "FlatMap", array.FlatMap([]int{1, 2}, func(v int) []int { return []int{v, v} }), []int{1, 1, 2, 2})
	eq(t, "Uniq", array.Uniq([]int{3, 1, 3, 2, 1}), []int{3, 1, 2})
	eq(t, "Compact", array.Compact([]string{"a", "", "b", ""}), []string{"a", "b"})
	eq(t, "CompactBy", array.CompactBy(in, func(v int) bool { return v == 2 }), []int{1, 3, 4})
	eq(t, "Reduce", array.Reduce(in, func(acc string, v int) string { return acc + fmt.Sprint(v) }, ">"), ">12234")
	eq(t, "Sum", array.Sum([]float64{1.5, 2.5}), 4.0)
	eq(t, "SumBy", array.SumBy([]string{"ab", "c"}, func(s string) int { return len(s) }), 3)
	eq(t, "Map(nil)", array.Map[int, int](nil, nil), []int{})
}

func TestUniqLargeAndBy(t *testing.T) {
	in := make([]int, 1000)
	for i := range in {
		in[i] = i % 37
	}
	got := array.Uniq(in)
	if len(got) != 37 || got[0] != 0 || got[36] != 36 {
		t.Fatalf("Uniq large: %v", got)
	}
	words := []string{"apple", "avocado", "banana", "blueberry", "cherry"}
	eq(t, "UniqBy", array.UniqBy(words, func(s string) byte { return s[0] }), []string{"apple", "banana", "cherry"})
}

func TestModifyDoesNotMutate(t *testing.T) {
	in := []string{"a", "b", "c", "b"}
	orig := slices.Clone(in)

	eq(t, "RemoveByValue", array.RemoveByValue(in, "b"), []string{"a", "c", "b"})
	eq(t, "RemoveByValue missing", array.RemoveByValue(in, "z"), in)
	eq(t, "RemoveAll", array.RemoveAll(in, "b"), []string{"a", "c"})
	got, err := array.RemoveByIndex(in, 0)
	eq(t, "RemoveByIndex", got, []string{"b", "c", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := array.RemoveByIndex(in, 4); !errors.Is(err, array.ErrIndexOutOfRange) {
		t.Fatalf("RemoveByIndex OOR: %v", err)
	}
	eq(t, "input", in, orig)
}

func TestTakeDrop(t *testing.T) {
	s := []int{1, 2, 3, 4}
	got, err := array.Take(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "Take", got, []int{1, 2})
	got, _ = array.Take(s, 10)
	eq(t, "Take all", got, s)
	if _, err := array.Take(s, -1); !errors.Is(err, array.ErrNegativeCount) {
		t.Fatalf("Take(-1): %v", err)
	}
	eq(t, "Drop", array.Drop(s, 1), []int{2, 3, 4})
	eq(t, "Drop neg", array.Drop(s, -1), s)
	eq(t, "Drop all", array.Drop(s, 9), []int{})
	eq(t, "DropRight", array.DropRight(s, 1), []int{1, 2, 3})
	eq(t, "DropRight all", array.DropRight(s, 9), []int{})
	lt3 := func(v int) bool { return v < 3 }
	eq(t, "DropWhile", array.DropWhile(s, lt3), []int{3, 4})
	eq(t, "DropWhile all", array.DropWhile(s, func(int) bool { return true }), []int{})
	eq(t, "TakeWhile", array.TakeWhile(s, lt3), []int{1, 2})
	eq(t, "TakeWhile all", array.TakeWhile(s, func(int) bool { return true }), s)

	// Results are copies: writing to them must not touch s.
	d := array.Drop(s, 0)
	d[0] = 99
	if s[0] != 1 {
		t.Fatal("Drop aliases input")
	}
}

func TestFill(t *testing.T) {
	s := []int{1, 2, 3}
	eq(t, "Fill", array.Fill(s, 9, 1, 3), []int{1, 9, 9})
	eq(t, "Fill clamp", array.Fill(s, 9, -5, 50), []int{9, 9, 9})
	eq(t, "Fill end<start", array.Fill(s, 9, 2, 1), []int{1, 2, 3})
	eq(t, "Fill start>len", array.Fill(s, 9, 5, 9), []int{1, 2, 3})
	eq(t, "Fill empty", array.Fill([]int{}, 9, 0, 1), []int{})
	eq(t, "input", s, []int{1, 2, 3})
}

func TestCombine(t *testing.T) {
	eq(t, "Concat", array.Concat([]int{1}, nil, []int{2, 3}), []int{1, 2, 3})
	eq(t, "Concat empty", array.Concat[int](), []int{})
	eq(t, "Interleave", array.Interleave([]int{1, 2, 3}, []int{10}, []int{20, 21}), []int{1, 10, 20, 2, 21, 3})
	eq(t, "Interleave none", array.Interleave[int](), []int{})
	zipped := array.Zip([]int{1, 2, 3}, []string{"a", "b"})
	eq(t, "Zip", zipped, []array.Pair[int, string]{{1, "a"}, {2, "b"}})
	nums, strs := array.Unzip(zipped)
	eq(t, "Unzip", [2]any{nums, strs}, [2]any{[]int{1, 2}, []string{"a", "b"}})
	eq(t, "ZipWith", array.ZipWith([]int{1, 2}, []int{3, 4, 5}, func(a, b int) int { return a + b }), []int{4, 6})
	eq(t, "ZipWithIndex", array.ZipWithIndex([]string{"x"}), []array.Pair[int, string]{{0, "x"}})
	eq(t, "GroupBy", array.GroupBy([]string{"aa", "b", "cc"}, func(s string) int { return len(s) }), map[int][]string{1: {"b"}, 2: {"aa", "cc"}})
	eq(t, "KeyBy", array.KeyBy([]string{"aa", "b", "cc"}, func(s string) int { return len(s) }), map[int]string{1: "b", 2: "cc"})
}

func TestPartition(t *testing.T) {
	pass, fail := array.Partition([]int{1, 2, 3, 4, 5, 6}, func(v int) bool { return v%2 == 0 })
	eq(t, "pass", pass, []int{2, 4, 6})
	eq(t, "fail", fail, []int{1, 3, 5})
	pass = append(pass, 100) // must not overwrite fail
	eq(t, "fail after append", fail, []int{1, 3, 5})

	groups := array.PartitionBy([]int{1, 1, 2, 2, 2, 1}, func(v int) int { return v })
	eq(t, "PartitionBy", groups, [][]int{{1, 1}, {2, 2, 2}, {1}})
	groups[0] = append(groups[0], 7) // must not overwrite groups[1]
	eq(t, "PartitionBy after append", groups[1], []int{2, 2, 2})
	eq(t, "PartitionBy empty", array.PartitionBy([]int{}, func(v int) int { return v }), [][]int{})
}

func TestChunkWindow(t *testing.T) {
	s := []int{1, 2, 3, 4, 5}
	chunks := array.Chunk(s, 2)
	eq(t, "Chunk", chunks, [][]int{{1, 2}, {3, 4}, {5}})
	chunks[0] = append(chunks[0], 99) // clipped: must not overwrite s[2]
	if s[2] != 3 {
		t.Fatalf("Chunk append corrupted input: %v", s)
	}
	eq(t, "Chunk 0", array.Chunk(s, 0), [][]int{})
	eq(t, "Chunk big", array.Chunk(s, 10), [][]int{{1, 2, 3, 4, 5}})

	if _, err := array.Split(s, 0); !errors.Is(err, array.ErrInvalidSize) {
		t.Fatalf("Split(0): %v", err)
	}
	split, err := array.Split(s, 3)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "Split", split, [][]int{{1, 2, 3}, {4, 5}})

	win := array.Window(s, 3)
	eq(t, "Window", win, [][]int{{1, 2, 3}, {2, 3, 4}, {3, 4, 5}})
	win[0] = append(win[0], 42)
	if s[3] != 4 {
		t.Fatalf("Window append corrupted input: %v", s)
	}
	eq(t, "Window too big", array.Window(s, 6), [][]int{})
	eq(t, "Window full", array.Window(s, 5), [][]int{s})
}

func TestOrder(t *testing.T) {
	in := []int{3, 1, 2}
	eq(t, "Sort", array.Sort(in, cmp.Compare[int]), []int{1, 2, 3})
	eq(t, "SortLess", array.SortLess(in, func(a, b int) bool { return a > b }), []int{3, 2, 1})
	type kv struct {
		K string
		V int
	}
	kvs := []kv{{"b", 1}, {"a", 1}, {"c", 0}}
	eq(t, "SortBy", array.SortBy(kvs, func(x kv) int { return x.V }), []kv{{"c", 0}, {"b", 1}, {"a", 1}})
	eq(t, "SortStable", array.SortStable(kvs, func(a, b kv) int { return cmp.Compare(a.V, b.V) }), []kv{{"c", 0}, {"b", 1}, {"a", 1}})
	eq(t, "input", in, []int{3, 1, 2})

	eq(t, "Reverse", array.Reverse(in), []int{2, 1, 3})
	eq(t, "Rotate", array.Rotate([]int{1, 2, 3, 4}, 1), []int{2, 3, 4, 1})
	eq(t, "Rotate neg", array.Rotate([]int{1, 2, 3, 4}, -1), []int{4, 1, 2, 3})
	eq(t, "Rotate 0", array.Rotate([]int{1, 2}, 4), []int{1, 2})
	eq(t, "Rotate empty", array.Rotate([]int{}, 3), []int{})

	lo, hi, err := array.MinMax([]int{4, 1, 9, 2})
	if err != nil || lo != 1 || hi != 9 {
		t.Fatalf("MinMax = %d %d %v", lo, hi, err)
	}
	if _, _, err := array.MinMax[int](nil); !errors.Is(err, array.ErrEmpty) {
		t.Fatalf("MinMax empty: %v", err)
	}
	flo, fhi, _ := array.MinMax([]float64{1, math.NaN(), 3})
	if !math.IsNaN(flo) || !math.IsNaN(fhi) {
		t.Fatalf("MinMax NaN = %v %v", flo, fhi)
	}
}

func TestShuffle(t *testing.T) {
	in := []int{1, 2, 3, 4, 5, 6, 7, 8}
	got := array.Shuffle(in)
	if !array.AreIdentical(in, got) {
		t.Fatalf("Shuffle changed elements: %v", got)
	}
	a := array.ShuffleWith(in, rand.New(rand.NewPCG(1, 1)))
	b := array.ShuffleWith(in, rand.New(rand.NewPCG(1, 1)))
	eq(t, "seeded shuffle", a, b)
	eq(t, "input", in, []int{1, 2, 3, 4, 5, 6, 7, 8})
}

func TestQuery(t *testing.T) {
	s := []int{1, 2, 3, 4}
	even := func(v int) bool { return v%2 == 0 }
	if !array.Contains(s, 3) || array.Contains(s, 9) {
		t.Fatal("Contains")
	}
	if array.IndexOf(s, 3) != 2 || array.IndexOf(s, 9) != -1 {
		t.Fatal("IndexOf")
	}
	if v, ok := array.Find(s, even); !ok || v != 2 {
		t.Fatalf("Find = %v %v", v, ok)
	}
	if v, ok := array.FindLast(s, even); !ok || v != 4 {
		t.Fatalf("FindLast = %v %v", v, ok)
	}
	if _, ok := array.Find(s, func(v int) bool { return v > 9 }); ok {
		t.Fatal("Find miss")
	}
	if _, ok := array.FindLast(s, func(v int) bool { return v > 9 }); ok {
		t.Fatal("FindLast miss")
	}
	if array.Count(s, even) != 2 {
		t.Fatal("Count")
	}
	if array.Every(s, even) || !array.Every([]int{}, even) || !array.Every([]int{2}, even) {
		t.Fatal("Every")
	}
	if !array.Some(s, even) || array.Some([]int{1}, even) {
		t.Fatal("Some")
	}
	if array.None(s, even) || !array.None([]int{1}, even) {
		t.Fatal("None")
	}
	if !array.AreIdentical([]int{1, 2, 2}, []int{2, 1, 2}) || array.AreIdentical([]int{1, 2}, []int{1, 1}) || array.AreIdentical([]int{1}, []int{1, 1}) {
		t.Fatal("AreIdentical")
	}
	eq(t, "DistinctCount", array.DistinctCount([]string{"a", "b", "a"}), map[string]int{"a": 2, "b": 1})
}

type name string

func (n name) String() string { return "<" + string(n) + ">" }

type myErr struct{}

func (myErr) Error() string  { return "err" }
func (myErr) String() string { return "stringer" } // fmt prefers Error

func TestJoin(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{array.Join([]int{1, -2, 3}, ","), "1,-2,3"},
		{array.Join([]string{"a"}, ","), "a"},
		{array.Join([]int{}, ","), ""},
		{array.Join([]float64{1.5, 1e21, 0.1}, " "), fmt.Sprint(1.5) + " " + fmt.Sprint(1e21) + " " + fmt.Sprint(0.1)},
		{array.Join([]float32{0.1}, ""), fmt.Sprint(float32(0.1))},
		{array.Join([]bool{true, false}, "|"), "true|false"},
		{array.Join([]uint8{1, 255}, ","), "1,255"},
		{array.Join([]name{"x", "y"}, "-"), "<x>-<y>"},
		{array.Join([]myErr{{}}, ""), "err"},
		{array.Join([]any{1, "a", nil, []int{1}}, ","), "1,a,<nil>,[1]"},
		{array.Join([]struct{ A int }{{1}}, ""), "{1}"},
		{array.JoinString([]name{"a", "b"}, "+"), "<a>+<b>"},
		{array.JoinString([]name{"a"}, "+"), "<a>"},
		{array.JoinString([]name{}, "+"), ""},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q, want %q", i, c.got, c.want)
		}
	}
	// Join must match fmt for every integer type.
	ints := []any{int8(-8), int16(-16), int32(-32), int64(-64), uint(1), uint16(16), uint32(32), uint64(64)}
	for _, v := range ints {
		if got := array.Join([]any{v}, ""); got != fmt.Sprint(v) {
			t.Errorf("Join(%T) = %q", v, got)
		}
	}
}

func TestFlattenDeep(t *testing.T) {
	eq(t, "Flatten", array.Flatten([][]int{{1, 2}, {}, {3}}), []int{1, 2, 3})
	eq(t, "FlattenDeep", array.FlattenDeep([]any{1, []any{2, []int{3, 4}}, [2]string{"a", "b"}, "cd"}), []any{1, 2, 3, 4, "a", "b", "cd"})
	eq(t, "FlattenDeep nil", array.FlattenDeep(nil), []any{})
	eq(t, "FlattenDeep scalar", array.FlattenDeep(5), []any{5})
	eq(t, "FlattenDeep nil elem", array.FlattenDeep([]any{nil, 1}), []any{nil, 1})

	// A slice that contains itself must terminate.
	cyclic := []any{1, nil}
	cyclic[1] = cyclic
	eq(t, "FlattenDeep cycle", array.FlattenDeep(cyclic), []any{1})

	// Very deep nesting is cut off instead of exhausting the stack.
	var deep any = 1
	for range 10_000 {
		deep = []any{deep}
	}
	if got := array.FlattenDeep(deep); len(got) != 1 {
		t.Fatalf("deep: %d leaves", len(got))
	}
}

func TestSeq(t *testing.T) {
	seq := slices.Values([]int{1, 2, 2, 3, 4, 5, 6})
	got := slices.Collect(array.TakeSeq(array.MapSeq(array.FilterSeq(array.UniqSeq(seq), func(v int) bool { return v%2 == 0 }), func(v int) int { return v * 10 }), 2))
	eq(t, "pipeline", got, []int{20, 40})
	eq(t, "TakeSeq 0", slices.Collect(array.TakeSeq(seq, 0)), []int(nil))

	// Early stop must propagate through every stage.
	for v := range array.MapSeq(array.FilterSeq(array.UniqSeq(seq), func(int) bool { return true }), func(v int) int { return v }) {
		if v == 2 {
			break
		}
	}
}

func TestJoinFastPathsMatchFmt(t *testing.T) {
	check := func(got string, vals ...any) {
		t.Helper()
		want := ""
		for i, v := range vals {
			if i > 0 {
				want += ";"
			}
			want += fmt.Sprint(v)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	check(array.Join([]int{-1, 1000}, ";"), -1, 1000)
	check(array.Join([]int64{math.MinInt64}, ";"), int64(math.MinInt64))
	check(array.Join([]int32{-7, 7}, ";"), int32(-7), int32(7))
	check(array.Join([]uint64{math.MaxUint64}, ";"), uint64(math.MaxUint64))
	check(array.Join([]uint32{4}, ";"), uint32(4))
	check(array.Join([]float64{1e-7, 2.5, math.Inf(1)}, ";"), 1e-7, 2.5, math.Inf(1))
	check(array.Join([]string{"a", "b"}, ";"), "a", "b")
}
