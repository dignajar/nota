// Package timeparse turns human input like "2h", "tomorrow 9am" or
// "next friday 14:30" into an absolute time.
package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultHour is used when a day is given without a time ("tomorrow").
const DefaultHour = 9

// Duration parses relative amounts: "2h", "1h30m", "90m", "2d", "1w",
// "2 hours", "an hour", "1 day and 2 hours", "half an hour".
func Duration(s string) (time.Duration, error) {
	in := normalize(s)
	in = strings.TrimPrefix(in, "in ")
	in = strings.TrimSuffix(in, " from now")
	if in == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasPrefix(in, "half an hour") || in == "half hour" {
		return 30 * time.Minute, nil
	}
	in = unitDigitRe.ReplaceAllString(in, "$1 $2") // "1h30m" -> "1h 30m"
	matches := durRe.FindAllStringSubmatchIndex(in, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("not a duration: %q", s)
	}
	var total time.Duration
	pos := 0
	for _, m := range matches {
		if gap := strings.TrimSpace(in[pos:m[0]]); gap != "" && gap != "and" && gap != "," {
			return 0, fmt.Errorf("not a duration: %q", s)
		}
		numStr, unit := strings.TrimSpace(in[m[2]:m[3]]), in[m[4]:m[5]]
		var num float64
		if numStr == "a" || numStr == "an" {
			num = 1
		} else {
			var err error
			if num, err = strconv.ParseFloat(numStr, 64); err != nil {
				return 0, fmt.Errorf("bad number %q", numStr)
			}
		}
		total += time.Duration(num * float64(unitDur(unit)))
		pos = m[1]
	}
	if rest := strings.Trim(strings.TrimSpace(in[pos:]), ","); rest != "" {
		return 0, fmt.Errorf("not a duration: %q", s)
	}
	if total <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return total, nil
}

var (
	durRe       = regexp.MustCompile(`(\d+(?:\.\d+)?\s*|\ban?\s+)(seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d|weeks?|wks?|w)\b`)
	unitDigitRe = regexp.MustCompile(`([a-z])(\d)`)
)

func unitDur(u string) time.Duration {
	switch u[0] {
	case 's':
		return time.Second
	case 'm':
		return time.Minute
	case 'h':
		return time.Hour
	case 'd':
		return 24 * time.Hour
	default: // w
		return 7 * 24 * time.Hour
	}
}

// Parse resolves s relative to now. It accepts durations ("2h"), absolute
// dates ("2026-10-20 14:00"), times ("9am", "17:30"), day words ("today",
// "tonight", "tomorrow", weekdays, "next week") and combinations in either
// order ("tomorrow 9am", "9am tomorrow", "friday at noon").
// The result must be in the future.
func Parse(s string, now time.Time) (time.Time, error) {
	in := normalize(s)
	if in == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if d, err := Duration(in); err == nil {
		return now.Add(d), nil
	}
	if t, ok := parseAbsolute(in, now.Location()); ok {
		if !t.After(now) {
			return time.Time{}, fmt.Errorf("%q is in the past", s)
		}
		return t, nil
	}

	words := strings.Fields(in)
	var (
		day      *time.Time
		dayWord  string
		hour     = -1
		minute   = 0
		explicit bool // a day word was given
	)
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch w {
		case "at", "on", "in", "the", "this", "by":
			continue
		}
		// "9 pm" written with a space
		if i+1 < len(words) && (words[i+1] == "am" || words[i+1] == "pm") {
			w += words[i+1]
			i++
		}
		if h, m, ok := parseClock(w); ok {
			if hour >= 0 {
				return time.Time{}, fmt.Errorf("two times in %q", s)
			}
			hour, minute = h, m
			continue
		}
		if w == "next" && i+1 < len(words) {
			if words[i+1] == "week" {
				d := startOfDay(now).AddDate(0, 0, daysUntil(now.Weekday(), time.Monday, true))
				day, dayWord, explicit = &d, "next week", true
				i++
				continue
			}
			if wd, ok := weekdays[words[i+1]]; ok {
				d := startOfDay(now).AddDate(0, 0, daysUntil(now.Weekday(), wd, true))
				day, dayWord, explicit = &d, words[i+1], true
				i++
				continue
			}
		}
		if d, ok := dayFromWord(w, now); ok {
			if day != nil {
				return time.Time{}, fmt.Errorf("two days in %q", s)
			}
			day, dayWord, explicit = &d, w, true
			if h, ok := impliedHour[w]; ok && hour < 0 {
				hour = h
			}
			continue
		}
		if h, ok := impliedHour[w]; ok { // "morning", "evening", "eod"
			if hour < 0 {
				hour = h
			}
			continue
		}
		return time.Time{}, fmt.Errorf("could not understand %q (try 2h, tomorrow 9am, friday 14:30, 2026-10-20)", s)
	}

	if day == nil && hour < 0 {
		return time.Time{}, fmt.Errorf("could not understand %q", s)
	}
	if hour < 0 {
		hour = DefaultHour
		if dayWord == "today" {
			hour = 17
		}
	}
	base := startOfDay(now)
	if day != nil {
		base = *day
	}
	t := time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, now.Location())
	if !explicit && !t.After(now) {
		t = t.AddDate(0, 0, 1) // bare "9am" already passed today: tomorrow
	}
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("%q is in the past", s)
	}
	return t, nil
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), " ")
}

var absLayouts = []string{
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func parseAbsolute(s string, loc *time.Location) (time.Time, bool) {
	s = strings.ToUpper(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(loc), true
	}
	for _, l := range absLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			if l == "2006-01-02" {
				t = t.Add(DefaultHour * time.Hour)
			}
			return t, true
		}
	}
	return time.Time{}, false
}

var clockRe = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm|a|p|h)?$`)

func parseClock(w string) (h, m int, ok bool) {
	switch w {
	case "noon", "midday":
		return 12, 0, true
	case "midnight":
		return 0, 0, true
	}
	mm := clockRe.FindStringSubmatch(w)
	if mm == nil {
		return 0, 0, false
	}
	// a bare number like "9" counts only when paired with am/pm or ":"
	if mm[2] == "" && mm[3] == "" {
		return 0, 0, false
	}
	h, _ = strconv.Atoi(mm[1])
	if mm[2] != "" {
		m, _ = strconv.Atoi(mm[2])
	}
	switch mm[3] {
	case "am", "a":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h == 12 {
			h = 0
		}
	case "pm", "p":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h != 12 {
			h += 12
		}
	}
	if h > 23 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "thurs": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

var impliedHour = map[string]int{
	"tonight":   20,
	"morning":   9,
	"afternoon": 14,
	"evening":   18,
	"night":     20,
	"eod":       17,
}

func dayFromWord(w string, now time.Time) (time.Time, bool) {
	today := startOfDay(now)
	switch w {
	case "today", "tonight", "eod":
		return today, true
	case "tomorrow", "tmr", "tmrw", "tomorow":
		return today.AddDate(0, 0, 1), true
	}
	if wd, ok := weekdays[w]; ok {
		return today.AddDate(0, 0, daysUntil(now.Weekday(), wd, false)), true
	}
	return time.Time{}, false
}

// daysUntil returns days from `from` to the next `to`. A weekday equal to
// today means next week. With nextWeek the target must be in the following
// calendar week (Monday based) rather than just the next occurrence.
func daysUntil(from, to time.Weekday, nextWeek bool) int {
	d := (int(to) - int(from) + 7) % 7
	if d == 0 {
		d = 7
	}
	if nextWeek {
		mondayOffset := (int(time.Monday) - int(from) + 7) % 7
		if mondayOffset == 0 {
			mondayOffset = 7
		}
		// target index within next week, Monday = 0
		idx := (int(to) + 6) % 7
		d = mondayOffset + idx
	}
	return d
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
