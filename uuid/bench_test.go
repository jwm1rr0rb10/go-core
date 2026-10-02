package uuid_test

import (
	"testing"

	"github.com/jwm1rr0rb10/go-core/uuid"
)

var sinkUUID uuid.UUID
var sinkString string

func BenchmarkNewV4(b *testing.B) {
	for b.Loop() {
		sinkUUID = uuid.NewV4()
	}
}

func BenchmarkNewV7(b *testing.B) {
	for b.Loop() {
		sinkUUID = uuid.NewV7()
	}
}

func BenchmarkNewV4Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var u uuid.UUID
		for pb.Next() {
			u = uuid.NewV4()
		}
		_ = u
	})
}

func BenchmarkNewV7Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var u uuid.UUID
		for pb.Next() {
			u = uuid.NewV7()
		}
		_ = u
	})
}

func BenchmarkString(b *testing.B) {
	u := uuid.NewV7()
	for b.Loop() {
		sinkString = u.String()
	}
}

func BenchmarkParse(b *testing.B) {
	s := uuid.NewV7().String()
	for b.Loop() {
		sinkUUID, _ = uuid.Parse(s)
	}
}
