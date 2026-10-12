package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/dignajar/nota/internal/store"
)

func TestShort(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{12 * time.Minute, "12m"},
		{90 * time.Minute, "1h30m"},
		{3 * time.Hour, "3h"},
		{20 * time.Hour, "20h"},
		{47 * time.Hour, "47h"},
		{3 * 24 * time.Hour, "3d"},
		{15 * 24 * time.Hour, "2w"},
		{-2 * time.Hour, "2h"}, // sign is dropped, callers add "ago"/"late"
	}
	for _, c := range cases {
		if got := short(c.d); got != c.want {
			t.Errorf("short(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestFullTime(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if got := fullTime(now.Add(2*time.Hour), now); got != "Mon 12 Oct 12:00 (in 2h)" {
		t.Errorf("future: %q", got)
	}
	if got := fullTime(now.Add(-30*time.Minute), now); got != "Mon 12 Oct 09:30 (30m ago)" {
		t.Errorf("past: %q", got)
	}
	if got := fullTime(now.AddDate(1, 0, 0), now); !strings.Contains(got, "2027") {
		t.Errorf("other year should show the year: %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("no-op: %q", got)
	}
	if got := truncate("hello world", 6); got != "hello…" {
		t.Errorf("ascii: %q", got)
	}
	if got := truncate("ñandú árbol", 5); got != "ñand…" {
		t.Errorf("must cut on runes, not bytes: %q", got)
	}
	if got := truncate("x", 0); got != "" {
		t.Errorf("zero width: %q", got)
	}
}

func TestWhenCell(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	var plain palette // zero value: no colors
	cases := []struct {
		name string
		n    store.Note
		want string
	}{
		{"done", store.Note{Status: store.StatusDone}, "done"},
		{"upcoming", store.Note{Status: store.StatusOpen, RemindAt: now.Add(2 * time.Hour)}, "in 2h"},
		{"late", store.Note{Status: store.StatusOpen, RemindAt: now.Add(-3 * time.Hour)}, "3h late"},
		{"no reminder", store.Note{Status: store.StatusOpen, Created: now.Add(-10 * time.Minute)}, "10m ago"},
	}
	for _, c := range cases {
		got, width := whenCell(&c.n, now, plain)
		if got != c.want || width != len(c.want) {
			t.Errorf("%s: got (%q, %d), want %q", c.name, got, width, c.want)
		}
	}
}
