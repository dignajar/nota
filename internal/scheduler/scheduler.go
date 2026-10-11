// Package scheduler fires notifications for due reminders.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/dignajar/nota/internal/store"
)

// Notifier sends a desktop notification.
type Notifier interface {
	Send(title, body string) error
}

// SummaryThreshold: when more than this many reminders are due at once
// (for example after the laptop was off for a week) one summary
// notification is sent instead of a flood.
const SummaryThreshold = 3

// Scheduler polls the notes folder and fires due reminders.
//
// It polls a cheap directory fingerprint instead of using a filesystem
// watcher or one long timer: Go timers use the monotonic clock, which does
// not advance while a laptop sleeps on macOS, so a timer set for "in 8h"
// before closing the lid would fire late. Short polls always see wall time.
type Scheduler struct {
	Store    *store.Store
	Notifier Notifier
	Interval time.Duration // max sleep between checks
	Now      func() time.Time
	Log      *log.Logger

	sig   string
	notes []*store.Note
}

// Due returns open, unnotified notes whose reminder time has passed,
// oldest first.
func Due(notes []*store.Note, now time.Time) []*store.Note {
	var due []*store.Note
	for _, n := range notes {
		if n.IsOpen() && n.HasReminder() && !n.Notified && !n.RemindAt.After(now) {
			due = append(due, n)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].RemindAt.Before(due[j].RemindAt) })
	return due
}

// NextDue returns the earliest pending reminder after now.
func NextDue(notes []*store.Note, now time.Time) (time.Time, bool) {
	var next time.Time
	for _, n := range notes {
		if !n.IsOpen() || !n.HasReminder() || n.Notified || !n.RemindAt.After(now) {
			continue
		}
		if next.IsZero() || n.RemindAt.Before(next) {
			next = n.RemindAt
		}
	}
	return next, !next.IsZero()
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log.Printf(format, args...)
	}
}

// refresh reloads notes only when the folder fingerprint changed.
func (s *Scheduler) refresh(force bool) error {
	sig, err := s.Store.Signature()
	if err != nil {
		return err
	}
	if !force && sig == s.sig && s.notes != nil {
		return nil
	}
	notes, warns := s.Store.LoadAll()
	for _, w := range warns {
		s.logf("warning: %v", w)
	}
	s.sig, s.notes = sig, notes
	return nil
}

// Tick fires everything that is due right now. It returns how many
// reminders were delivered.
func (s *Scheduler) Tick() (int, error) {
	if err := s.refresh(false); err != nil {
		return 0, err
	}
	due := Due(s.notes, s.now())
	if len(due) == 0 {
		return 0, nil
	}

	var sendErr error
	if len(due) > SummaryThreshold {
		var b strings.Builder
		for i, n := range due {
			if i == SummaryThreshold {
				fmt.Fprintf(&b, "and %d more (nota list --due)", len(due)-i)
				break
			}
			b.WriteString("• " + truncate(n.Title(), 60) + "\n")
		}
		sendErr = s.Notifier.Send(fmt.Sprintf("nota: %d reminders", len(due)), b.String())
	} else {
		for _, n := range due {
			if err := s.Notifier.Send(notificationTitle(n), notificationBody(n)); err != nil {
				sendErr = err
			}
		}
	}
	if sendErr != nil {
		// keep them unnotified so the next tick retries
		return 0, fmt.Errorf("notify: %w", sendErr)
	}

	for _, n := range due {
		_, err := s.Store.Update(n.ID, func(cur *store.Note) {
			// only mark if the reminder was not changed meanwhile
			if cur.RemindAt.Equal(n.RemindAt) {
				cur.Notified = true
			}
		})
		if err != nil {
			s.logf("mark %s notified: %v", n.ID, err)
			continue
		}
		s.logf("reminded %s: %s", n.ID, n.Title())
	}
	return len(due), s.refresh(true)
}

// Run loops until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.Interval <= 0 {
		s.Interval = 5 * time.Second
	}
	s.logf("watching %s", s.Store.NotesDir())
	for {
		if _, err := s.Tick(); err != nil {
			s.logf("error: %v", err)
		}
		sleep := s.Interval
		if next, ok := NextDue(s.notes, s.now()); ok {
			if d := next.Sub(s.now()); d < sleep {
				sleep = d
			}
		}
		if sleep < 100*time.Millisecond {
			sleep = 100 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			s.logf("stopping")
			return nil
		case <-time.After(sleep):
		}
	}
}

func notificationTitle(n *store.Note) string {
	if c := n.Context(); c != "" {
		return "nota · " + c
	}
	return "nota"
}

func notificationBody(n *store.Note) string {
	return truncate(strings.TrimSpace(n.Body), 240)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
