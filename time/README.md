# time

[Русская версия](READMEru.md) · [← go-core](../README.md)

Helpers that complement the standard `time` package: Unix timestamps whose
unit is not known in advance, cached time zone loading, fixed offsets with
minutes (+5:30, +5:45), compact duration formatting and operation timing via
`log/slog`.

```bash
go get github.com/jwm1rr0rb10/go-core
```

The package name shadows the standard library, so import it with an alias:

```go
import coretime "github.com/jwm1rr0rb10/go-core/time"
```

## Features

- `UnixToTime` detects seconds / milliseconds / microseconds / nanoseconds by
  the number of digits, including negative (pre-1970) timestamps.
- `LoadLocation` caches `time.LoadLocation`: ≈10 ns instead of ≈4.5 µs and
  24 allocations per call. All timezone helpers use it.
- `LocationByOffsetMinutes` builds fixed zones such as `UTC+5:30` or `UTC-9:30`.
- `FormatDuration` prints `850ns`, `12µs`, `340ms`, `1.50s`, `1h2m3s`; negative
  durations get a minus sign.
- `Track` measures an operation and logs it through `slog` with a typed level.
- `CountDigitsInNumber` is exact over the whole `int64` range.

## API

| Function | Description |
|----------|-------------|
| `UnixToTime(ts) time.Time` | Timestamp of unknown unit → `time.Time` |
| `SecondsSince(ts) int` / `SecondsUntil(ts) int` | Rounded seconds from / to a timestamp |
| `UnixMilli() int64` | Current Unix time in milliseconds |
| `CountDigitsInNumber(n) int` | Decimal digits of `n`, sign ignored |
| `FormatDuration(d) string` | Compact single-unit duration |
| `Track(logger, level, name) func() time.Duration` | Start timing; call the result to log and get the duration |
| `LoadLocation(name)` | Cached `time.LoadLocation` |
| `ConvertToTimezone(t, tz)` / `NowInTimezone(tz)` | Time in an IANA zone |
| `ParseInTimezone(value, layout, tz)` | `time.ParseInLocation` with a zone name |
| `FormatWithTimezone(t, layout, tz)` | Convert and format |
| `LocationByOffset(hours)` | Fixed zone `UTC±H`, hours in [-12, 14] |
| `LocationByOffsetMinutes(minutes)` | Fixed zone `UTC±H[:MM]`, minutes in [-720, 840] |
| `TimeTrack(start, name, level...)` | **Deprecated**, use `Track` |

## Usage

```go
// Timestamps from different producers.
t1 := coretime.UnixToTime(1_767_225_600)     // seconds
t2 := coretime.UnixToTime(1_767_225_600_000) // milliseconds
fmt.Println(t1.Equal(t2)) // true

// Time zones; the zone is loaded once and then served from the cache.
s, err := coretime.FormatWithTimezone(time.Now(), time.DateTime, "Asia/Tokyo")

// Offsets with minutes.
nepal, _ := coretime.LocationByOffsetMinutes(5*60 + 45) // UTC+5:45

// Timing an operation.
func loadUsers(ctx context.Context) error {
	defer coretime.Track(logger, slog.LevelDebug, "load users")()
	// ...
}
```

## Concurrency and performance

All functions are safe for concurrent use. Measured on the test machine:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `time.LoadLocation("Europe/Moscow")` | 4493 | 3664 | 24 |
| `coretime.LoadLocation("Europe/Moscow")` | 10 | 0 | 0 |
| `UnixToTime` | 14 | 0 | 0 |
| `FormatDuration(1.5s)` | 64 | 16 | 2 |

The zone cache stores only successfully loaded zones, so it is bounded by the
size of the tz database even if zone names come from user input.

`UnixToTime` is a heuristic: a small value in a fine unit (for example
5 000 ms after the epoch) is read as seconds. When the unit is known, use
`time.Unix`, `time.UnixMilli` or `time.UnixMicro`.

## Migration

- `CountDigitsInNumber(math.MinInt64)` now returns 19 instead of a wrong value
  caused by overflow.
- `UnixToTime` with negative millisecond/microsecond/nanosecond timestamps now
  picks the unit by the absolute value.
- `FormatDuration` of negative durations returns `-1.50s` instead of
  `-1500000000ns`.
- Timezone errors now wrap the underlying `time.LoadLocation` error (the
  message still starts with `timeutil: invalid timezone "<name>"`).
- `TimeTrack` is deprecated in favour of `Track`, which logs via `slog` with an
  `slog.Level` instead of a string.
