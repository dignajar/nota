package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/dignajar/nota/internal/store"
)

type fakeNotifier struct {
	sent []string
	fail bool
}

func (f *fakeNotifier) Send(title, body string) error {
	if f.fail {
		return errors.New("boom")
	}
	f.sent = append(f.sent, title+"|"+body)
	return nil
}

var base = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

func TestDueAndNext(t *testing.T) {
	notes := []*store.Note{
		{ID: "a", Status: store.StatusOpen, RemindAt: base.Add(-time.Minute)},
		{ID: "b", Status: store.StatusOpen, RemindAt: base.Add(time.Hour)},
		{ID: "c", Status: store.StatusDone, RemindAt: base.Add(-time.Hour)},
		{ID: "d", Status: store.StatusOpen, RemindAt: base.Add(-time.Hour), Notified: true},
		{ID: "e", Status: store.StatusOpen},
		{ID: "f", Status: store.StatusOpen, RemindAt: base.Add(-2 * time.Minute)},
		{ID: "g", Status: store.StatusOpen, RemindAt: base.Add(30 * time.Minute)},
	}
	due := Due(notes, base)
	if len(due) != 2 || due[0].ID != "f" || due[1].ID != "a" {
		t.Fatalf("due = %v", ids(due))
	}
	next, ok := NextDue(notes, base)
	if !ok || !next.Equal(base.Add(30*time.Minute)) {
		t.Fatalf("next = %v %v", next, ok)
	}
}

func ids(ns []*store.Note) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func newSched(t *testing.T, now *time.Time) (*Scheduler, *fakeNotifier) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeNotifier{}
	return &Scheduler{Store: s, Notifier: f, Now: func() time.Time { return *now }}, f
}

func TestTickFiresOnceAndMarks(t *testing.T) {
	now := base
	sc, f := newSched(t, &now)
	n := &store.Note{Body: "check indexes", RemindAt: base.Add(time.Minute), GitRepo: "api", GitBranch: "main"}
	if err := sc.Store.Create(n); err != nil {
		t.Fatal(err)
	}

	if c, _ := sc.Tick(); c != 0 {
		t.Fatalf("fired early: %d", c)
	}
	now = base.Add(2 * time.Minute)
	if c, err := sc.Tick(); c != 1 || err != nil {
		t.Fatalf("tick = %d %v", c, err)
	}
	if f.sent[0] != "nota · api@main|check indexes" {
		t.Fatalf("sent %q", f.sent[0])
	}
	if c, _ := sc.Tick(); c != 0 {
		t.Fatal("fired twice")
	}
	got, _ := sc.Store.Get(n.ID)
	if !got.Notified {
		t.Fatal("not marked notified")
	}
}

func TestTickRetriesOnFailure(t *testing.T) {
	now := base
	sc, f := newSched(t, &now)
	sc.Store.Create(&store.Note{Body: "x", RemindAt: base.Add(-time.Second)})
	f.fail = true
	if _, err := sc.Tick(); err == nil {
		t.Fatal("expected error")
	}
	f.fail = false
	if c, _ := sc.Tick(); c != 1 {
		t.Fatal("did not retry")
	}
}

func TestTickSummary(t *testing.T) {
	now := base
	sc, f := newSched(t, &now)
	for i := 0; i < 5; i++ {
		sc.Store.Create(&store.Note{Body: "old", RemindAt: base.Add(-time.Duration(i+1) * time.Hour)})
	}
	if c, _ := sc.Tick(); c != 5 {
		t.Fatalf("count %d", c)
	}
	if len(f.sent) != 1 {
		t.Fatalf("expected one summary, got %d", len(f.sent))
	}
}

func TestTickPicksUpExternalEdit(t *testing.T) {
	now := base
	sc, f := newSched(t, &now)
	sc.Tick() // prime cache with empty dir
	n := &store.Note{Body: "added later", RemindAt: base.Add(-time.Second)}
	sc.Store.Create(n)
	if c, _ := sc.Tick(); c != 1 || len(f.sent) != 1 {
		t.Fatalf("new file not noticed: %d", c)
	}
}
