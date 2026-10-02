package array

import (
	"fmt"
	"strconv"
	"strings"
)

// Join formats each element like fmt's %v and concatenates them with sep.
//
// Common types (string, []byte, integers, floats, bool, error, fmt.Stringer)
// are formatted without reflection; other types fall back to fmt.
func Join[T any](s []T, sep string) string {
	if len(s) == 0 {
		return ""
	}
	// Fast paths for common element types avoid boxing each element.
	switch ss := any(s).(type) {
	case []string:
		return strings.Join(ss, sep)
	case []int:
		return joinInts(ss, sep)
	case []int64:
		return joinInts(ss, sep)
	case []int32:
		return joinInts(ss, sep)
	case []uint64:
		return joinUints(ss, sep)
	case []uint32:
		return joinUints(ss, sep)
	case []float64:
		return joinFloats(ss, sep)
	}
	var b strings.Builder
	b.Grow(len(s) * (len(sep) + 4))
	var scratch [32]byte
	for i, v := range s {
		if i > 0 {
			b.WriteString(sep)
		}
		writeValueBuf(&b, any(v), scratch[:0])
	}
	return b.String()
}

// writeValueBuf mirrors fmt's %v for the types it handles: error and
// fmt.Stringer take precedence, exactly as in fmt.
func writeValueBuf(b *strings.Builder, v any, buf []byte) {
	switch x := v.(type) {
	case error:
		b.WriteString(x.Error())
	case fmt.Stringer:
		b.WriteString(x.String())
	case string:
		b.WriteString(x)
	case int:
		b.Write(strconv.AppendInt(buf, int64(x), 10))
	case int8:
		b.Write(strconv.AppendInt(buf, int64(x), 10))
	case int16:
		b.Write(strconv.AppendInt(buf, int64(x), 10))
	case int32:
		b.Write(strconv.AppendInt(buf, int64(x), 10))
	case int64:
		b.Write(strconv.AppendInt(buf, x, 10))
	case uint:
		b.Write(strconv.AppendUint(buf, uint64(x), 10))
	case uint8:
		b.Write(strconv.AppendUint(buf, uint64(x), 10))
	case uint16:
		b.Write(strconv.AppendUint(buf, uint64(x), 10))
	case uint32:
		b.Write(strconv.AppendUint(buf, uint64(x), 10))
	case uint64:
		b.Write(strconv.AppendUint(buf, x, 10))
	case float64:
		b.Write(strconv.AppendFloat(buf, x, 'g', -1, 64))
	case float32:
		b.Write(strconv.AppendFloat(buf, float64(x), 'g', -1, 32))
	case bool:
		b.Write(strconv.AppendBool(buf, x))
	default:
		fmt.Fprint(b, v)
	}
}

// JoinString concatenates the String() of each element with sep.
func JoinString[T fmt.Stringer](s []T, sep string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0].String()
	}
	var b strings.Builder
	for i, v := range s {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(v.String())
	}
	return b.String()
}

func joinInts[T int | int32 | int64](s []T, sep string) string {
	buf := make([]byte, 0, len(s)*(len(sep)+4))
	for i, v := range s {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = strconv.AppendInt(buf, int64(v), 10)
	}
	return string(buf)
}

func joinUints[T uint32 | uint64](s []T, sep string) string {
	buf := make([]byte, 0, len(s)*(len(sep)+4))
	for i, v := range s {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = strconv.AppendUint(buf, uint64(v), 10)
	}
	return string(buf)
}

func joinFloats(s []float64, sep string) string {
	buf := make([]byte, 0, len(s)*(len(sep)+8))
	for i, v := range s {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = strconv.AppendFloat(buf, v, 'g', -1, 64)
	}
	return string(buf)
}
