package random_test

import (
	"testing"

	"github.com/jwm1rr0rb10/go-core/random"
)

var sinkS string
var sinkI int

func BenchmarkRandInt(b *testing.B) {
	for b.Loop() {
		sinkI, _ = random.RandInt(nil, 0, 1000)
	}
}

func BenchmarkRandIntParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = random.RandInt(nil, 0, 1000)
		}
	})
}

func BenchmarkRandString32(b *testing.B) {
	for b.Loop() {
		sinkS, _ = random.RandString(nil, 32, nil)
	}
}

func BenchmarkRandString32Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = random.RandString(nil, 32, nil)
		}
	})
}

func BenchmarkSecureString32(b *testing.B) {
	for b.Loop() {
		sinkS, _ = random.SecureString(32, nil)
	}
}

func BenchmarkSecureInt(b *testing.B) {
	for b.Loop() {
		sinkI, _ = random.SecureInt(0, 1000)
	}
}
