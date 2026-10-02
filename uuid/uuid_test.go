package uuid_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jwm1rr0rb10/go-core/uuid"
)

const sample = "0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f"

func TestParseForms(t *testing.T) {
	want := uuid.MustParse(sample)
	for _, s := range []string{
		sample,
		strings.ToUpper(sample),
		"{" + sample + "}",
		"urn:uuid:" + sample,
		"URN:UUID:" + sample,
		strings.ReplaceAll(sample, "-", ""),
	} {
		got, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %v, want %v", s, got, want)
		}
	}
	if got := want.String(); got != sample {
		t.Fatalf("String() = %q", got)
	}
	if got := want.URN(); got != "urn:uuid:"+sample {
		t.Fatalf("URN() = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]error{
		"":                                      uuid.ErrInvalidLength,
		"123":                                   uuid.ErrInvalidLength,
		"0190a3c2x7b1e-7c3a-9f2d-4b6e8a1c0d3f":  uuid.ErrInvalidFormat,
		"0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3g":  uuid.ErrInvalidFormat,
		"(" + sample + ")":                      uuid.ErrInvalidFormat,
		"urn:uuix:" + sample:                    uuid.ErrInvalidFormat,
		"zz90a3c27b1e7c3a9f2d4b6e8a1c0d3f":      uuid.ErrInvalidFormat,
		"0190a3c2-7b1e-7c3a-9f2d-4b6e8a1c0d3f0": uuid.ErrInvalidLength,
	}
	for in, want := range cases {
		if _, err := uuid.Parse(in); !errors.Is(err, want) {
			t.Errorf("Parse(%q) err = %v, want %v", in, err, want)
		}
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	uuid.MustParse("bad")
}

func TestVersionVariantTime(t *testing.T) {
	v4 := uuid.NewV4()
	if v4.Version() != 4 || v4.Variant() != uuid.VariantRFC9562 {
		t.Fatalf("v4: version=%d variant=%v", v4.Version(), v4.Variant())
	}
	if _, ok := v4.Time(); ok {
		t.Fatal("v4 must not report a time")
	}

	before := time.Now().Truncate(time.Millisecond)
	v7 := uuid.NewV7()
	after := time.Now()
	if v7.Version() != 7 || v7.Variant() != uuid.VariantRFC9562 {
		t.Fatalf("v7: version=%d variant=%v", v7.Version(), v7.Variant())
	}
	ts, ok := v7.Time()
	if !ok || ts.Before(before) || ts.After(after) {
		t.Fatalf("v7 time %v not in [%v, %v]", ts, before, after)
	}

	variants := map[byte]uuid.Variant{0x00: uuid.VariantNCS, 0x80: uuid.VariantRFC9562, 0xc0: uuid.VariantMicrosoft, 0xe0: uuid.VariantFuture}
	for b, want := range variants {
		var u uuid.UUID
		u[8] = b
		if got := u.Variant(); got != want || got.String() == "" {
			t.Errorf("variant(%#x) = %v, want %v", b, got, want)
		}
	}
}

func TestZeroCompareBytes(t *testing.T) {
	if !uuid.Nil.IsZero() || uuid.Max.IsZero() {
		t.Fatal("IsZero")
	}
	if uuid.Nil.Compare(uuid.Max) != -1 || uuid.Max.Compare(uuid.Nil) != 1 || uuid.Max.Compare(uuid.Max) != 0 {
		t.Fatal("Compare")
	}
	u := uuid.NewV4()
	b := u.Bytes()
	b[0] ^= 0xff // must be a copy
	got, err := uuid.FromBytes(u.Bytes())
	if err != nil || got != u {
		t.Fatalf("FromBytes: %v %v", got, err)
	}
	if _, err := uuid.FromBytes(b[:3]); !errors.Is(err, uuid.ErrInvalidLength) {
		t.Fatalf("FromBytes short: %v", err)
	}
}

func TestTextBinaryJSON(t *testing.T) {
	u := uuid.NewV7()

	txt, _ := u.MarshalText()
	var fromTxt uuid.UUID
	if err := fromTxt.UnmarshalText(txt); err != nil || fromTxt != u {
		t.Fatalf("text roundtrip: %v %v", fromTxt, err)
	}
	if app, _ := u.AppendText([]byte("id=")); string(app) != "id="+u.String() {
		t.Fatalf("AppendText: %q", app)
	}
	if err := fromTxt.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("expected text error")
	}

	bin, _ := u.MarshalBinary()
	var fromBin uuid.UUID
	if err := fromBin.UnmarshalBinary(bin); err != nil || fromBin != u {
		t.Fatalf("binary roundtrip: %v %v", fromBin, err)
	}
	if err := fromBin.UnmarshalBinary(bin[:5]); err == nil {
		t.Fatal("expected binary error")
	}

	type doc struct {
		ID  uuid.UUID  `json:"id"`
		Ref *uuid.UUID `json:"ref,omitempty"`
	}
	raw, err := json.Marshal(doc{ID: u})
	if err != nil || !bytes.Equal(raw, []byte(`{"id":"`+u.String()+`"}`)) {
		t.Fatalf("json marshal: %s %v", raw, err)
	}
	var back doc
	if err := json.Unmarshal(raw, &back); err != nil || back.ID != u {
		t.Fatalf("json unmarshal: %v %v", back, err)
	}
}

func TestSQL(t *testing.T) {
	u := uuid.NewV4()
	v, err := u.Value()
	if err != nil || v != u.String() {
		t.Fatalf("Value: %v %v", v, err)
	}
	for _, src := range []any{u.String(), []byte(u.String()), u.Bytes()} {
		var got uuid.UUID
		if err := got.Scan(src); err != nil || got != u {
			t.Fatalf("Scan(%T): %v %v", src, got, err)
		}
	}
	got := u
	if err := got.Scan(nil); err != nil || got != uuid.Nil {
		t.Fatalf("Scan(nil): %v %v", got, err)
	}
	if err := got.Scan(42); err == nil {
		t.Fatal("Scan(int) must fail")
	}

	var n uuid.NullUUID
	if err := n.Scan(nil); err != nil || n.Valid {
		t.Fatalf("NullUUID nil: %+v %v", n, err)
	}
	if v, _ := n.Value(); v != nil {
		t.Fatalf("NullUUID Value = %v", v)
	}
	if err := n.Scan(u.String()); err != nil || !n.Valid || n.UUID != u {
		t.Fatalf("NullUUID scan: %+v %v", n, err)
	}
	if v, _ := n.Value(); v != u.String() {
		t.Fatalf("NullUUID Value = %v", v)
	}
	if err := n.Scan("bad"); err == nil || n.Valid {
		t.Fatalf("NullUUID bad: %+v %v", n, err)
	}
}

func TestV7MonotonicSameMillisecond(t *testing.T) {
	fixed := time.UnixMilli(1_700_000_000_000)
	g := uuid.NewGenerator(uuid.WithClock(func() time.Time { return fixed }))

	// 10k values in one millisecond overflow the 12-bit counter several
	// times; ordering must survive by carrying into the timestamp.
	prev, _ := g.NewV7()
	for range 10_000 {
		next, err := g.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		if next.Compare(prev) <= 0 {
			t.Fatalf("not increasing: %v then %v", prev, next)
		}
		if next.Version() != 7 || next.Variant() != uuid.VariantRFC9562 {
			t.Fatalf("bad bits: %v", next)
		}
		prev = next
	}
	if ts, _ := prev.Time(); !ts.After(fixed) {
		t.Fatalf("expected timestamp to advance past %v on overflow, got %v", fixed, ts)
	}
}

func TestV7ClockGoesBackwards(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	g := uuid.NewGenerator(uuid.WithClock(func() time.Time { return now }))
	a, _ := g.NewV7()
	now = now.Add(-time.Hour)
	b, _ := g.NewV7()
	if b.Compare(a) <= 0 {
		t.Fatalf("clock step back broke ordering: %v then %v", a, b)
	}
}

func TestV7NewMillisecondUsesRealTime(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	g := uuid.NewGenerator(uuid.WithClock(func() time.Time { return now }))
	_, _ = g.NewV7()
	now = now.Add(5 * time.Millisecond)
	u, _ := g.NewV7()
	if ts, _ := u.Time(); !ts.Equal(now) {
		t.Fatalf("time = %v, want %v", ts, now)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestGeneratorReaderError(t *testing.T) {
	g := uuid.NewGenerator(uuid.WithReader(failingReader{}))
	if _, err := g.NewV4(); err == nil {
		t.Fatal("NewV4: expected error")
	}
	if _, err := g.NewV7(); err == nil {
		t.Fatal("NewV7: expected error")
	}
}

func TestGeneratorReaderDeterministic(t *testing.T) {
	g := uuid.NewGenerator(uuid.WithReader(bytes.NewReader(bytes.Repeat([]byte{0xff}, 16))))
	u, err := g.NewV4()
	if err != nil || u.Version() != 4 || u.Variant() != uuid.VariantRFC9562 {
		t.Fatalf("NewV4: %v %v", u, err)
	}
}

// concurrentUnique runs gen from several goroutines and fails on duplicates.
// When ordered is true, each goroutine's sequence must also be increasing.
func concurrentUnique(t *testing.T, gen func() uuid.UUID, ordered bool) {
	t.Helper()
	workers, per := 8, 100_000
	if testing.Short() {
		per = 10_000
	}
	results := make([][]uuid.UUID, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			ids := make([]uuid.UUID, per)
			for i := range ids {
				ids[i] = gen()
			}
			results[w] = ids
		})
	}
	wg.Wait()

	all := slices.Concat(results...)
	for w, ids := range results {
		if ordered && !slices.IsSortedFunc(ids, uuid.UUID.Compare) {
			t.Fatalf("worker %d: sequence not ordered", w)
		}
	}
	slices.SortFunc(all, uuid.UUID.Compare)
	for i := 1; i < len(all); i++ {
		if all[i] == all[i-1] {
			t.Fatalf("duplicate UUID %v", all[i])
		}
	}
}

func TestConcurrentUniqueV4(t *testing.T) { concurrentUnique(t, uuid.NewV4, false) }
func TestConcurrentUniqueV7(t *testing.T) { concurrentUnique(t, uuid.NewV7, true) }

func FuzzParse(f *testing.F) {
	f.Add(sample)
	f.Add("{" + sample + "}")
	f.Add("urn:uuid:" + sample)
	f.Add(strings.ReplaceAll(sample, "-", ""))
	f.Fuzz(func(t *testing.T, s string) {
		u, err := uuid.Parse(s)
		if err != nil {
			return
		}
		back, err := uuid.Parse(u.String())
		if err != nil || back != u {
			t.Fatalf("roundtrip of %q failed: %v %v", s, back, err)
		}
	})
}
