package pointer_test

import (
	"testing"

	"github.com/jwm1rr0rb10/go-core/pointer"
)

func TestToPointer(t *testing.T) {
	p := pointer.ToPointer(10)
	if p == nil || *p != 10 {
		t.Fatalf("got %v", p)
	}
	v := 1
	q := pointer.ToPointer(v)
	*q = 2
	if v != 1 {
		t.Fatal("ToPointer must copy")
	}
}

func TestToPointerOrNil(t *testing.T) {
	if pointer.ToPointerOrNil(0) != nil || pointer.ToPointerOrNil("") != nil {
		t.Fatal("zero values must map to nil")
	}
	if p := pointer.ToPointerOrNil("x"); p == nil || *p != "x" {
		t.Fatalf("got %v", p)
	}
}

func TestReaders(t *testing.T) {
	v := 42
	if got, ok := pointer.FromPointer(&v); !ok || got != 42 {
		t.Fatal("FromPointer non-nil")
	}
	if got, ok := pointer.FromPointer[int](nil); ok || got != 0 {
		t.Fatal("FromPointer nil")
	}
	if pointer.Deref(&v) != 42 || pointer.Deref[int](nil) != 0 {
		t.Fatal("Deref")
	}
	if pointer.ValueOr(&v, 7) != 42 || pointer.ValueOr(nil, 7) != 7 {
		t.Fatal("ValueOr")
	}
	if pointer.FromPointerOr(&v, 7) != 42 || pointer.FromPointerOr(nil, 7) != 7 {
		t.Fatal("FromPointerOr")
	}
}

func TestCoalesce(t *testing.T) {
	a, b := 1, 2
	if got := pointer.Coalesce(nil, &a, &b); got != &a {
		t.Fatal("Coalesce should return first non-nil")
	}
	if pointer.Coalesce[int](nil, nil) != nil || pointer.Coalesce[int]() != nil {
		t.Fatal("Coalesce of nils should be nil")
	}
}

func TestEqual(t *testing.T) {
	a, b, c := 1, 1, 2
	cases := []struct {
		x, y *int
		want bool
	}{
		{nil, nil, true}, {&a, nil, false}, {nil, &a, false},
		{&a, &b, true}, {&a, &c, false}, {&a, &a, true},
	}
	for i, tc := range cases {
		if got := pointer.Equal(tc.x, tc.y); got != tc.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestIsNil(t *testing.T) {
	v := 1
	if !pointer.IsNil[int](nil) || pointer.IsNil(&v) {
		t.Fatal("IsNil")
	}
}

func TestSwap(t *testing.T) {
	a, b := 1, 2
	pointer.Swap(&a, &b)
	if a != 2 || b != 1 {
		t.Fatal("Swap")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Swap(nil) must panic")
		}
	}()
	pointer.Swap(&a, nil)
}

func TestCloneAndCopy(t *testing.T) {
	s := "hello"
	c := pointer.Clone(&s)
	s = "changed"
	if *c != "hello" {
		t.Fatal("Clone must be independent")
	}
	if pointer.Clone[int](nil) != nil {
		t.Fatal("Clone(nil)")
	}

	// Non-comparable types are accepted now.
	sl := []int{1, 2}
	cp, err := pointer.Copy(&sl)
	if err != nil || len(*cp) != 2 {
		t.Fatalf("Copy slice: %v %v", cp, err)
	}
	if cp, err := pointer.Copy[int](nil); cp != nil || err != nil {
		t.Fatal("Copy(nil)")
	}
}

func TestSet(t *testing.T) {
	v := 1
	if !pointer.Set(&v, 9) || v != 9 {
		t.Fatal("Set non-nil")
	}
	if pointer.Set(nil, 9) {
		t.Fatal("Set nil")
	}
}
