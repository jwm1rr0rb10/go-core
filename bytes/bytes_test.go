package bytes_test

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	corebytes "github.com/jwm1rr0rb10/go-core/bytes"
)

type user struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func TestMapRoundTrip(t *testing.T) {
	raw, err := corebytes.MapToJSON(map[string]any{"name": "alice", "age": 30})
	if err != nil {
		t.Fatal(err)
	}
	got, err := corebytes.JSONToMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "alice" || got["age"].(float64) != 30 {
		t.Fatalf("got %v", got)
	}
}

func TestMapNil(t *testing.T) {
	raw, err := corebytes.MapToJSON(nil)
	if err != nil || string(raw) != "null" {
		t.Fatalf("got %s %v", raw, err)
	}
	m, err := corebytes.JSONToMap([]byte("null"))
	if err != nil || m != nil {
		t.Fatalf("null should decode to nil map: %v %v", m, err)
	}
}

func TestJSONToMapUseNumberKeepsPrecision(t *testing.T) {
	in := []byte(`{"id": 9007199254740993}`) // 2^53 + 1

	lossy, _ := corebytes.JSONToMap(in)
	if lossy["id"].(float64) != 9007199254740992 {
		t.Fatal("expected float64 rounding without UseNumber")
	}

	exact, err := corebytes.JSONToMap(in, corebytes.UseNumber())
	if err != nil {
		t.Fatal(err)
	}
	if n := exact["id"].(json.Number); n.String() != "9007199254740993" {
		t.Fatalf("got %v", n)
	}
}

func TestJSONToMapErrors(t *testing.T) {
	for _, in := range []string{"{", "[1,2]", "", `{"a":1} {"b":2}`, `"str"`} {
		if _, err := corebytes.JSONToMap([]byte(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
		if _, err := corebytes.JSONToMap([]byte(in), corebytes.UseNumber()); err == nil {
			t.Errorf("%q with UseNumber: expected error", in)
		}
	}
}

func TestUnmarshalTrailingDataWithOptions(t *testing.T) {
	_, err := corebytes.Unmarshal[user]([]byte(`{"id":1} garbage`), corebytes.UseNumber())
	if !errors.Is(err, corebytes.ErrTrailingData) {
		t.Fatalf("want ErrTrailingData, got %v", err)
	}
	_, err = corebytes.Unmarshal[user]([]byte(`  `), corebytes.UseNumber())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want ErrUnexpectedEOF, got %v", err)
	}
	if _, err := corebytes.Unmarshal[user]([]byte("{\"id\":1}\n\t")); err != nil {
		t.Fatalf("trailing whitespace must be accepted: %v", err)
	}
}

func TestGenericRoundTrip(t *testing.T) {
	raw, err := corebytes.Marshal(user{ID: 7, Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := corebytes.Unmarshal[user](raw)
	if err != nil || u != (user{ID: 7, Name: "bob"}) {
		t.Fatalf("got %+v %v", u, err)
	}
}

func TestDisallowUnknownFields(t *testing.T) {
	in := []byte(`{"id":1,"extra":true}`)
	if _, err := corebytes.Unmarshal[user](in); err != nil {
		t.Fatalf("default must ignore unknown fields: %v", err)
	}
	if _, err := corebytes.Unmarshal[user](in, corebytes.DisallowUnknownFields()); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDeprecatedWrappers(t *testing.T) {
	raw, err := corebytes.MapToByteArr(map[string]any{"a": "b"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := corebytes.ByteArrToMap(string(raw))
	if err != nil || m["a"] != "b" {
		t.Fatalf("got %v %v", m, err)
	}
	if _, err := corebytes.ByteArrToMap("{"); err == nil {
		t.Fatal("expected error")
	}
}
