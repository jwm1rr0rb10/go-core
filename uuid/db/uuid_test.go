package db_test

import (
	"sync"
	"testing"

	"github.com/jwm1rr0rb10/go-core/uuid"
	"github.com/jwm1rr0rb10/go-core/uuid/db"
)

func TestVersions(t *testing.T) {
	v4, err := db.NewV4()
	if err != nil || v4.Version() != 4 || v4.Variant() != uuid.VariantRFC9562 {
		t.Fatalf("v4: %v %v", v4, err)
	}
	v7, err := db.NewV7()
	if err != nil || v7.Version() != 7 || v7.Variant() != uuid.VariantRFC9562 {
		t.Fatalf("v7: %v %v", v7, err)
	}
	var _ uuid.UUID = v7 // same type as the root package
}

func TestConcurrentUniqueOrderedV7(t *testing.T) {
	workers, per := 8, 100_000
	if testing.Short() {
		per = 10_000
	}
	var mu sync.Mutex
	seen := make(map[db.UUID]struct{}, workers*per)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			local := make([]db.UUID, per)
			for i := range local {
				local[i], _ = db.NewV7()
				if i > 0 && local[i].Compare(local[i-1]) <= 0 {
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

func BenchmarkNewV7(b *testing.B) {
	for b.Loop() {
		_, _ = db.NewV7()
	}
}

func BenchmarkNewV7Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = db.NewV7()
		}
	})
}
