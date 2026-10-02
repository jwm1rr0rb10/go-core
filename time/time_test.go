package time_test

import (
	"bytes"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	coretime "github.com/jwm1rr0rb10/go-core/time"
)

func TestCountDigitsInNumber(t *testing.T) {
	cases := map[int64]int{
		0: 1, 9: 1, 10: 2, -12345: 5, 999_999_999_999: 12,
		math.MaxInt64: 19, math.MinInt64: 19, -1: 1,
		9_999_999_999_999_999: 16, 99_999_999_999_999_999: 17,
	}
	for n, want := range cases {
		if got := coretime.CountDigitsInNumber(n); got != want {
			t.Errorf("CountDigitsInNumber(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestUnixToTimeScales(t *testing.T) {
	ref := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)
	cases := []struct {
		name string
		in   int64
		want time.Time
	}{
		{"seconds", ref.Unix(), time.Unix(ref.Unix(), 0)},
		{"millis", ref.UnixMilli(), time.UnixMilli(ref.UnixMilli())},
		{"micros", ref.UnixMicro(), time.UnixMicro(ref.UnixMicro())},
		{"nanos", ref.UnixNano(), ref},
		{"negative seconds", -86400, time.Unix(-86400, 0)},
		{"negative millis", -86_400_000_000, time.UnixMilli(-86_400_000_000)},
	}
	for _, c := range cases {
		if got := coretime.UnixToTime(c.in); !got.Equal(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSecondsSinceAndUntil(t *testing.T) {
	past := time.Now().Add(-3 * time.Second).Unix()
	if s := coretime.SecondsSince(past); s < 2 || s > 5 {
		t.Fatalf("SecondsSince = %d", s)
	}
	future := time.Now().Add(3 * time.Second).UnixMilli()
	if s := coretime.SecondsUntil(future); s < 1 || s > 4 {
		t.Fatalf("SecondsUntil = %d", s)
	}
}

func TestUnixMilli(t *testing.T) {
	before := time.Now().UnixMilli()
	got := coretime.UnixMilli()
	if got < before || got > time.Now().UnixMilli() {
		t.Fatalf("UnixMilli out of range: %d", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                            "0ns",
		500 * time.Nanosecond:        "500ns",
		12 * time.Microsecond:        "12µs",
		500 * time.Millisecond:       "500ms",
		1500 * time.Millisecond:      "1.50s",
		61500 * time.Millisecond:     "1m2s",
		-1500 * time.Millisecond:     "-1.50s",
		time.Duration(math.MinInt64): time.Duration(math.MinInt64).Round(time.Second).String(),
	}
	for d, want := range cases {
		if got := coretime.FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTrack(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	stop := coretime.Track(logger, slog.LevelDebug, "load users")
	time.Sleep(time.Millisecond)
	if d := stop(); d < time.Millisecond {
		t.Fatalf("elapsed %v", d)
	}
	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "operation=\"load users\"") {
		t.Fatalf("unexpected log line: %s", out)
	}
	coretime.Track(nil, slog.LevelDebug, "default logger")() // must not panic
}

func TestTimeTrackCompat(t *testing.T) {
	coretime.TimeTrack(time.Now(), "legacy")
	coretime.TimeTrack(time.Now(), "legacy", "DEBUG")
}

func TestTimezoneHelpers(t *testing.T) {
	utc := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	tokyo, err := coretime.ConvertToTimezone(utc, "Asia/Tokyo")
	if err != nil || tokyo.Location().String() != "Asia/Tokyo" || tokyo.Hour() != 21 {
		t.Fatalf("ConvertToTimezone: %v %v", tokyo, err)
	}

	now, err := coretime.NowInTimezone("UTC")
	if err != nil || now.Location().String() != "UTC" {
		t.Fatalf("NowInTimezone: %v %v", now, err)
	}

	parsed, err := coretime.ParseInTimezone("2026-07-11 15:04", "2006-01-02 15:04", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if _, off := parsed.Zone(); off != 2*3600 {
		t.Fatalf("Berlin summer offset = %d", off)
	}

	s, err := coretime.FormatWithTimezone(utc, time.RFC3339, "Asia/Kolkata")
	if err != nil || s != "2026-07-11T17:30:00+05:30" {
		t.Fatalf("FormatWithTimezone = %q %v", s, err)
	}

	for _, f := range []func() error{
		func() error { _, err := coretime.ConvertToTimezone(utc, "Mars/Base"); return err },
		func() error { _, err := coretime.NowInTimezone("Mars/Base"); return err },
		func() error { _, err := coretime.ParseInTimezone("x", "y", "Mars/Base"); return err },
		func() error { _, err := coretime.FormatWithTimezone(utc, "", "Mars/Base"); return err },
	} {
		if err := f(); err == nil || !strings.Contains(err.Error(), "Mars/Base") {
			t.Fatalf("expected invalid timezone error, got %v", err)
		}
	}
}

func TestLoadLocationCachesAndIsConcurrent(t *testing.T) {
	a, err := coretime.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			b, err := coretime.LoadLocation("America/New_York")
			if err != nil || b != a {
				t.Errorf("cache miss or error: %p %p %v", a, b, err)
			}
		})
	}
	wg.Wait()
}

func TestLocationByOffset(t *testing.T) {
	loc, err := coretime.LocationByOffset(3)
	if err != nil || loc.String() != "UTC+3" {
		t.Fatalf("got %v %v", loc, err)
	}
	if loc, _ := coretime.LocationByOffset(-5); loc.String() != "UTC-5" {
		t.Fatalf("got %v", loc)
	}
	for _, bad := range []int{-13, 15, 99} {
		if _, err := coretime.LocationByOffset(bad); err == nil {
			t.Fatalf("offset %d accepted", bad)
		}
	}
}

func TestLocationByOffsetMinutes(t *testing.T) {
	cases := map[int]string{330: "UTC+5:30", 345: "UTC+5:45", -570: "UTC-9:30", 0: "UTC+0", 840: "UTC+14"}
	ref := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for minutes, name := range cases {
		loc, err := coretime.LocationByOffsetMinutes(minutes)
		if err != nil || loc.String() != name {
			t.Fatalf("%d: got %v %v, want %s", minutes, loc, err, name)
		}
		if _, off := ref.In(loc).Zone(); off != minutes*60 {
			t.Fatalf("%d: offset %d", minutes, off)
		}
	}
	if _, err := coretime.LocationByOffsetMinutes(841); err == nil {
		t.Fatal("expected range error")
	}
}
