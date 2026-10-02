package google_uuid_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	idgen "github.com/jwm1rr0rb10/go-core/uuid/google_uuid"
)

func TestGoogleUUIDGenerators(t *testing.T) {
	for want, gen := range map[uuid.Version]idgen.IDGenerator{
		4: idgen.NewGoogleUUIDGenerator(),
		7: idgen.NewGoogleUUIDv7Generator(),
	} {
		id := gen.GenerateID()
		u, err := uuid.Parse(id)
		if err != nil || u.Version() != want {
			t.Fatalf("v%d: %q err=%v version=%d", want, id, err, u.Version())
		}
	}
}

func TestULIDFormatAndOrder(t *testing.T) {
	gen := idgen.NewULIDGenerator()
	prev := gen.GenerateID()
	for range 10_000 {
		id := gen.GenerateID()
		if len(id) != ulid.EncodedSize || id != strings.ToUpper(id) {
			t.Fatalf("bad ulid %q", id)
		}
		if _, err := ulid.ParseStrict(id); err != nil {
			t.Fatalf("parse %q: %v", id, err)
		}
		if id <= prev { // Crockford base32 sorts lexicographically
			t.Fatalf("not monotonic: %q then %q", prev, id)
		}
		prev = id
	}
}

func TestConcurrentUnique(t *testing.T) {
	workers, per := 8, 100_000
	if testing.Short() {
		per = 10_000
	}
	gens := map[string]idgen.IDGenerator{
		"ulid":  idgen.NewULIDGenerator(),
		"uuid4": idgen.NewGoogleUUIDGenerator(),
		"uuid7": idgen.NewGoogleUUIDv7Generator(),
	}
	for name, gen := range gens {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			seen := make(map[string]struct{}, workers*per)
			var wg sync.WaitGroup
			for range workers {
				wg.Go(func() {
					local := make([]string, per)
					for i := range local {
						local[i] = gen.GenerateID()
					}
					mu.Lock()
					defer mu.Unlock()
					for _, id := range local {
						if _, dup := seen[id]; dup {
							t.Errorf("duplicate %q", id)
							return
						}
						seen[id] = struct{}{}
					}
				})
			}
			wg.Wait()
		})
	}
}

func BenchmarkULID(b *testing.B) {
	gen := idgen.NewULIDGenerator()
	for b.Loop() {
		_ = gen.GenerateID()
	}
}

func BenchmarkULIDParallel(b *testing.B) {
	gen := idgen.NewULIDGenerator()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.GenerateID()
		}
	})
}

func BenchmarkGoogleUUIDv4(b *testing.B) {
	gen := idgen.NewGoogleUUIDGenerator()
	for b.Loop() {
		_ = gen.GenerateID()
	}
}

func BenchmarkGoogleUUIDv7(b *testing.B) {
	gen := idgen.NewGoogleUUIDv7Generator()
	for b.Loop() {
		_ = gen.GenerateID()
	}
}
