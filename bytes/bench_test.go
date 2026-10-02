package bytes_test

import (
	"testing"

	corebytes "github.com/jwm1rr0rb10/go-core/bytes"
)

var doc = []byte(`{"id":123456789,"name":"alice","tags":["a","b"],"active":true}`)

func BenchmarkJSONToMap(b *testing.B) {
	for b.Loop() {
		if _, err := corebytes.JSONToMap(doc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONToMapUseNumber(b *testing.B) {
	for b.Loop() {
		if _, err := corebytes.JSONToMap(doc, corebytes.UseNumber()); err != nil {
			b.Fatal(err)
		}
	}
}
