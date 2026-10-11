package timeparse

import (
	"testing"
	"time"
)

// Monday 12 October 2026, 10:00.
var now = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

func at(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func TestParse(t *testing.T) {
	cases := map[string]time.Time{
		"2h":                   now.Add(2 * time.Hour),
		"in 30m":               now.Add(30 * time.Minute),
		"1h30m":                now.Add(90 * time.Minute),
		"2 hours":              now.Add(2 * time.Hour),
		"an hour":              now.Add(time.Hour),
		"1 day and 2 hours":    now.Add(26 * time.Hour),
		"half an hour":         now.Add(30 * time.Minute),
		"2d":                   now.Add(48 * time.Hour),
		"1w":                   now.Add(7 * 24 * time.Hour),
		"tomorrow":             at(2026, 10, 13, 9, 0),
		"tomorrow 9am":         at(2026, 10, 13, 9, 0),
		"9am tomorrow":         at(2026, 10, 13, 9, 0),
		"tomorrow at 14:30":    at(2026, 10, 13, 14, 30),
		"Tomorrow 2:15pm":      at(2026, 10, 13, 14, 15),
		"tomorrow 9 pm":        at(2026, 10, 13, 21, 0),
		"today 17:00":          at(2026, 10, 12, 17, 0),
		"today":                at(2026, 10, 12, 17, 0),
		"tonight":              at(2026, 10, 12, 20, 0),
		"eod":                  at(2026, 10, 12, 17, 0),
		"3pm":                  at(2026, 10, 12, 15, 0),
		"9am":                  at(2026, 10, 13, 9, 0), // already passed today
		"noon":                 at(2026, 10, 12, 12, 0),
		"friday":               at(2026, 10, 16, 9, 0),
		"fri 16:00":            at(2026, 10, 16, 16, 0),
		"monday":               at(2026, 10, 19, 9, 0), // today is monday: next one
		"next monday":          at(2026, 10, 19, 9, 0),
		"next friday":          at(2026, 10, 23, 9, 0),
		"next week":            at(2026, 10, 19, 9, 0),
		"wednesday evening":    at(2026, 10, 14, 18, 0),
		"tomorrow morning":     at(2026, 10, 13, 9, 0),
		"2026-10-20":           at(2026, 10, 20, 9, 0),
		"2026-10-20 14:00":     at(2026, 10, 20, 14, 0),
		"2026-10-20T08:15":     at(2026, 10, 20, 8, 15),
		"2026-10-20T08:15:00Z": at(2026, 10, 20, 8, 15),
	}
	for in, want := range cases {
		got, err := Parse(in, now)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("%q: got %s want %s", in, got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "banana", "today 8am", "2020-01-01", "25:00", "13pm", "tomorrow friday", "9", "2h banana"} {
		if got, err := Parse(in, now); err == nil {
			t.Errorf("%q: expected error, got %s", in, got)
		}
	}
}
