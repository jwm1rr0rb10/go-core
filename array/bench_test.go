package array_test

import (
	"strings"
	"testing"

	"github.com/jwm1rr0rb10/go-core/array"
)

var benchInts = func() []int {
	s := make([]int, 10_000)
	for i := range s {
		s[i] = i % 1000
	}
	return s
}()

func BenchmarkMap(b *testing.B) {
	for b.Loop() {
		_ = array.Map(benchInts, func(v int) int { return v * 2 })
	}
}

func BenchmarkFilter(b *testing.B) {
	for b.Loop() {
		_ = array.Filter(benchInts, func(v int) bool { return v%2 == 0 })
	}
}

func BenchmarkUniq(b *testing.B) {
	for b.Loop() {
		_ = array.Uniq(benchInts)
	}
}

func BenchmarkUniqSmall(b *testing.B) {
	s := benchInts[:10]
	for b.Loop() {
		_ = array.Uniq(s)
	}
}

func BenchmarkChunk(b *testing.B) {
	for b.Loop() {
		_ = array.Chunk(benchInts, 100)
	}
}

func BenchmarkPartition(b *testing.B) {
	for b.Loop() {
		_, _ = array.Partition(benchInts, func(v int) bool { return v%2 == 0 })
	}
}

func BenchmarkJoinInts(b *testing.B) {
	s := benchInts[:1000]
	for b.Loop() {
		_ = array.Join(s, ",")
	}
}

func BenchmarkJoinStrings(b *testing.B) {
	s := strings.Fields(strings.Repeat("alpha beta gamma delta ", 250))
	for b.Loop() {
		_ = array.Join(s, ",")
	}
}
