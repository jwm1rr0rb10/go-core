package bytes

import (
	stdbytes "bytes"
	"encoding/json"
	"io"

	"github.com/jwm1rr0rb10/go-errors"
)

// ErrTrailingData is returned when the input holds more than one JSON value.
var ErrTrailingData = errors.New("bytes: unexpected data after top-level JSON value")

// DecodeOption configures Unmarshal and JSONToMap.
type DecodeOption func(*decodeConfig)

type decodeConfig struct {
	useNumber      bool
	disallowFields bool
}

// UseNumber decodes numbers inside interface{} values as json.Number instead
// of float64, so integers above 2^53 (IDs, nanosecond timestamps) keep their
// exact value.
func UseNumber() DecodeOption {
	return func(c *decodeConfig) { c.useNumber = true }
}

// DisallowUnknownFields makes decoding into a struct fail when the input has
// a key that matches no exported field.
func DisallowUnknownFields() DecodeOption {
	return func(c *decodeConfig) { c.disallowFields = true }
}

// Marshal returns the JSON encoding of v. It is json.Marshal with a typed
// parameter, provided for symmetry with Unmarshal.
func Marshal[T any](v T) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal decodes data into a new value of type T.
//
// Without options it is json.Unmarshal. With options it uses a json.Decoder
// and still rejects anything after the first JSON value, like json.Unmarshal
// does (ErrTrailingData).
func Unmarshal[T any](data []byte, opts ...DecodeOption) (T, error) {
	var v T
	err := decode(data, &v, opts)
	return v, err
}

func decode(data []byte, v any, opts []DecodeOption) error {
	if len(opts) == 0 {
		return json.Unmarshal(data, v)
	}
	var cfg decodeConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	dec := json.NewDecoder(stdbytes.NewReader(data))
	if cfg.useNumber {
		dec.UseNumber()
	}
	if cfg.disallowFields {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		if err == io.EOF {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return ErrTrailingData
	}
	return nil
}

// MapToJSON returns the JSON encoding of m. A nil map encodes as null.
func MapToJSON(m map[string]any) ([]byte, error) {
	return json.Marshal(m)
}

// JSONToMap decodes a JSON object into a map. The input "null" yields a nil
// map and no error; any other non-object input is an error. Pass UseNumber()
// to keep numbers exact.
func JSONToMap(data []byte, opts ...DecodeOption) (map[string]any, error) {
	var m map[string]any
	if err := decode(data, &m, opts); err != nil {
		return nil, err
	}
	return m, nil
}

// MapToByteArr returns the JSON encoding of m.
//
// Deprecated: use MapToJSON.
func MapToByteArr(m map[string]any) ([]byte, error) {
	return MapToJSON(m)
}

// ByteArrToMap decodes a JSON object held in a string.
//
// Deprecated: use JSONToMap, which takes []byte and accepts options.
func ByteArrToMap(j string) (map[string]any, error) {
	return JSONToMap([]byte(j))
}
