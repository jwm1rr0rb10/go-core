package network_test

import (
	"sync"
	"testing"

	"github.com/jwm1rr0rb10/go-core/uuid"
	"github.com/jwm1rr0rb10/go-core/uuid/network"
)

func TestVersions(t *testing.T) {
	for range 1000 {
		v4 := network.NewV4()
		if v4.Version() != 4 || v4.Variant() != uuid.VariantRFC9562 {
			t.Fatalf("v4 bits: %v", v4)
		}
		v7 := network.NewV7()
		if v7.Version() != 7 || v7.Variant() != uuid.VariantRFC9562 {
			t.Fatalf("v7 bits: %v", v7)
		}
	}
}

func concurrentUnique(t *testing.T, gen func() network.UUID, ordered bool) {
	t.Helper()
	workers, per := 8, 100_000
	if testing.Short() {
		per = 10_000
	}
	var mu sync.Mutex
	seen := make(map[network.UUID]struct{}, workers*per)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			local := make([]network.UUID, per)
			for i := range local {
				local[i] = gen()
				if ordered && i > 0 && local[i].Compare(local[i-1]) <= 0 {
					t.Errorf("not monotonic: %v then %v", local[i-1], local[i])
					return
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, u := range local {
				if _, dup := seen[u]; dup {
					t.Errorf("duplicate %v", u)
					return
				}
				seen[u] = struct{}{}
			}
		})
	}
	wg.Wait()
}

func TestConcurrentUniqueV4(t *testing.T) { concurrentUnique(t, network.NewV4, false) }
func TestConcurrentUniqueV7(t *testing.T) { concurrentUnique(t, network.NewV7, true) }

func BenchmarkNewV4(b *testing.B) {
	for b.Loop() {
		network.NewV4()
	}
}

func BenchmarkNewV7(b *testing.B) {
	for b.Loop() {
		network.NewV7()
	}
}

func BenchmarkNewV4Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			network.NewV4()
		}
	})
}

func BenchmarkNewV7Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			network.NewV7()
		}
	})
}
