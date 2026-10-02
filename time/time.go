package time

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// CountDigitsInNumber returns the number of decimal digits in n, ignoring the
// sign. Zero has one digit. It is exact for the whole int64 range, including
// math.MinInt64.
func CountDigitsInNumber(n int64) int {
	u := uint64(n)
	if n < 0 {
		u = -u // two's complement magnitude; correct for MinInt64 too
	}
	digits := 1
	for u >= 10 {
		u /= 10
		digits++
	}
	return digits
}

// UnixToTime converts a Unix timestamp whose unit is not known in advance.
// The unit is guessed from the number of digits of the absolute value:
//
//	≤ 10 digits  seconds       (until year 2286)
//	≤ 13 digits  milliseconds
//	≤ 16 digits  microseconds
//	otherwise    nanoseconds
//
// Negative timestamps (before 1970) use the same rule. The guess is wrong for
// small values in fine units (for example 5_000 milliseconds after the
// epoch); when the unit is known, use time.Unix, time.UnixMilli or
// time.UnixMicro instead.
func UnixToTime(ts int64) time.Time {
	switch digits := CountDigitsInNumber(ts); {
	case digits <= 10:
		return time.Unix(ts, 0)
	case digits <= 13:
		return time.UnixMilli(ts)
	case digits <= 16:
		return time.UnixMicro(ts)
	default:
		return time.Unix(0, ts)
	}
}

// SecondsSince returns the number of seconds, rounded, from the timestamp to
// now. The timestamp unit is detected as in UnixToTime. The result is
// negative for timestamps in the future.
func SecondsSince(ts int64) int {
	return int(time.Since(UnixToTime(ts)).Round(time.Second) / time.Second)
}

// SecondsUntil returns the number of seconds, rounded, from now to the
// timestamp. The timestamp unit is detected as in UnixToTime. The result is
// negative for timestamps in the past.
func SecondsUntil(ts int64) int {
	return int(time.Until(UnixToTime(ts)).Round(time.Second) / time.Second)
}

// UnixMilli returns the current Unix time in milliseconds.
func UnixMilli() int64 { return time.Now().UnixMilli() }

// Track starts timing an operation and returns a function that logs its
// duration to logger at the given level and returns it. A nil logger means
// slog.Default(). Typical use:
//
//	defer coretime.Track(logger, slog.LevelDebug, "load users")()
func Track(logger *slog.Logger, level slog.Level, name string) func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		elapsed := time.Since(start)
		l := logger
		if l == nil {
			l = slog.Default()
		}
		l.LogAttrs(context.Background(), level, "operation finished",
			slog.String("operation", name),
			slog.Duration("elapsed", elapsed),
		)
		return elapsed
	}
}

// TimeTrack logs how long an operation took using the standard log package,
// as "[LEVEL] name took 1.2ms". level defaults to "INFO".
//
// Deprecated: use Track, which logs through log/slog with a typed level.
func TimeTrack(start time.Time, name string, level ...string) {
	logLevel := "INFO"
	if len(level) > 0 {
		logLevel = level[0]
	}
	log.Printf("[%s] %s took %s", logLevel, name, time.Since(start))
}

// FormatDuration renders d in a compact human-readable form using a single
// unit: "850ns", "12µs", "340ms", "1.50s" and, from one minute on, the
// standard form rounded to seconds ("1h2m3s"). Negative durations get a
// leading minus sign.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		if d == time.Duration(-1<<63) { // cannot be negated
			return d.Round(time.Second).String()
		}
		return "-" + FormatDuration(-d)
	}
	switch {
	case d < time.Microsecond:
		return strconv.FormatInt(d.Nanoseconds(), 10) + "ns"
	case d < time.Millisecond:
		return strconv.FormatInt(d.Microseconds(), 10) + "µs"
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
	default:
		return d.Round(time.Second).String()
	}
}

// locations caches successfully loaded zones. Only valid IANA names are
// stored, so the cache is bounded by the size of the tz database even when
// names come from untrusted input.
var locations sync.Map // map[string]*time.Location

// LoadLocation is a cached time.LoadLocation. The standard function reads and
// parses tzdata on every call; this one does it once per zone name, which
// makes repeated conversions several hundred times cheaper.
// Errors are not cached.
func LoadLocation(name string) (*time.Location, error) {
	if loc, ok := locations.Load(name); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("timeutil: invalid timezone %q: %w", name, err)
	}
	actual, _ := locations.LoadOrStore(name, loc)
	return actual.(*time.Location), nil
}

// ConvertToTimezone returns t in the named IANA time zone, for example
// "Asia/Tokyo".
func ConvertToTimezone(t time.Time, timezone string) (time.Time, error) {
	loc, err := LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	return t.In(loc), nil
}

// NowInTimezone returns the current time in the named IANA time zone.
func NowInTimezone(timezone string) (time.Time, error) {
	return ConvertToTimezone(time.Now(), timezone)
}

// ParseInTimezone parses value with layout, interpreting it in the named
// time zone when the value carries no offset of its own.
func ParseInTimezone(value, layout, timezone string) (time.Time, error) {
	loc, err := LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation(layout, value, loc)
}

// FormatWithTimezone formats t with layout after converting it to the named
// time zone.
func FormatWithTimezone(t time.Time, layout, timezone string) (string, error) {
	loc, err := LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	return t.In(loc).Format(layout), nil
}

// Offsets in use worldwide range from UTC-12:00 to UTC+14:00.
const (
	minOffsetMinutes = -12 * 60
	maxOffsetMinutes = 14 * 60
)

// LocationByOffset returns a fixed zone named like "UTC+3" for a whole-hour
// offset in [-12, 14]. For zones such as India (+5:30) or Nepal (+5:45) use
// LocationByOffsetMinutes.
func LocationByOffset(offsetHours int) (*time.Location, error) {
	if offsetHours < -12 || offsetHours > 14 {
		return nil, fmt.Errorf("timeutil: offset %d is out of range [-12, 14]", offsetHours)
	}
	return LocationByOffsetMinutes(offsetHours * 60)
}

// LocationByOffsetMinutes returns a fixed zone for an offset given in
// minutes east of UTC, in [-720, 840]. Whole hours are named "UTC+3",
// others "UTC+5:30" or "UTC-9:30".
func LocationByOffsetMinutes(offsetMinutes int) (*time.Location, error) {
	if offsetMinutes < minOffsetMinutes || offsetMinutes > maxOffsetMinutes {
		return nil, fmt.Errorf("timeutil: offset %d minutes is out of range [%d, %d]",
			offsetMinutes, minOffsetMinutes, maxOffsetMinutes)
	}
	sign := "+"
	abs := offsetMinutes
	if abs < 0 {
		sign, abs = "-", -abs
	}
	name := "UTC" + sign + strconv.Itoa(abs/60)
	if m := abs % 60; m != 0 {
		name += fmt.Sprintf(":%02d", m)
	}
	return time.FixedZone(name, offsetMinutes*60), nil
}
