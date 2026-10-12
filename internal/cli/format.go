package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dignajar/nota/internal/store"
)

type palette struct{ on bool }

func (p palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}
func (p palette) dim(s string) string    { return p.wrap("2", s) }
func (p palette) red(s string) string    { return p.wrap("31", s) }
func (p palette) yellow(s string) string { return p.wrap("33", s) }
func (p palette) cyan(s string) string   { return p.wrap("36", s) }
func (p palette) green(s string) string  { return p.wrap("32", s) }

// short renders a duration as 45s, 12m, 3h, 2d, 5w.
func short(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) % 60; h < 10 && m >= 5 {
			return fmt.Sprintf("%dh%02dm", h, m)
		}
		return strconv.Itoa(h) + "h"
	case d < 14*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	default:
		return strconv.Itoa(int(d.Hours()/24/7)) + "w"
	}
}

// fullTime renders an absolute time with its relative offset.
func fullTime(t, now time.Time) string {
	layout := "Mon 02 Jan 15:04"
	if t.Year() != now.Year() {
		layout = "Mon 02 Jan 2006 15:04"
	}
	rel := "in " + short(t.Sub(now))
	if !t.After(now) {
		rel = short(now.Sub(t)) + " ago"
	}
	return t.Format(layout) + " (" + rel + ")"
}

// whenCell is the WHEN column in list output (plain text, width ~9).
func whenCell(n *store.Note, now time.Time, p palette) (string, int) {
	var s string
	var colored string
	switch {
	case !n.IsOpen():
		s = "done"
		colored = p.green(s)
	case n.HasReminder() && n.RemindAt.After(now):
		s = "in " + short(n.RemindAt.Sub(now))
		colored = p.yellow(s)
	case n.HasReminder():
		s = short(now.Sub(n.RemindAt)) + " late"
		colored = p.red(s)
	default:
		s = short(now.Sub(n.Created)) + " ago"
		colored = p.dim(s)
	}
	return colored, utf8.RuneCountInString(s)
}

func termWidth() int {
	if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 20 {
		return c
	}
	return 100
}

func truncate(s string, max int) string {
	if max < 1 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func (a *App) printTable(notes []*store.Note) {
	now := a.now()
	width := termWidth()
	for _, n := range notes {
		when, wlen := whenCell(n, now, a.color)
		pad := strings.Repeat(" ", max(0, 10-wlen))

		var suffix []string
		lowerTitle := strings.ToLower(n.Title())
		for _, t := range n.Tags {
			if !strings.Contains(lowerTitle, "#"+t) { // already visible in the text
				suffix = append(suffix, "#"+t)
			}
		}
		if c := n.Context(); c != "" {
			suffix = append(suffix, "["+c+"]")
		}
		tail := strings.Join(suffix, " ")

		room := width - len(n.ID) - 2 - 10 - 1
		title := n.Title()
		if tail != "" {
			tailLen := utf8.RuneCountInString(tail) + 2
			if room-tailLen >= 20 {
				room -= tailLen
			} else {
				tail = ""
			}
		}
		title = truncate(title, room)
		if !n.IsOpen() {
			title = a.color.dim(title)
		}
		line := a.color.cyan(n.ID) + "  " + when + pad + " " + title
		if tail != "" {
			line += "  " + a.color.dim(tail)
		}
		fmt.Fprintln(a.Out, line)
	}
}

func (a *App) printNote(n *store.Note) {
	now := a.now()
	p := a.color
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(a.Out, "%s %s\n", p.dim(fmt.Sprintf("%-9s", k)), v)
		}
	}
	row("id", p.cyan(n.ID))
	row("status", string(n.Status))
	row("created", fullTime(n.Created, now))
	if n.HasReminder() {
		r := fullTime(n.RemindAt, now)
		if n.Notified {
			r += " notified"
		}
		row("remind", r)
	}
	if len(n.Tags) > 0 {
		row("tags", "#"+strings.Join(n.Tags, " #"))
	}
	row("repo", n.Context())
	row("dir", n.Cwd)
	row("file", n.Path)
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, n.Body)
}
