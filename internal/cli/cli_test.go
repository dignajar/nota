package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local)

type harness struct {
	t   *testing.T
	dir string
	cwd string
	now time.Time
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, dir: t.TempDir(), cwd: t.TempDir(), now: fixedNow}
}

func (h *harness) run(stdin string, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	a := &App{
		Out: &out, Err: &errb,
		In: strings.NewReader(stdin), StdinIsTTY: stdin == "",
		Now: func() time.Time { return h.now },
		Dir: h.dir, Cwd: h.cwd,
	}
	code := a.Run(args)
	return out.String(), errb.String(), code
}

func (h *harness) ok(args ...string) string {
	out, errs, code := h.run("", args...)
	if code != 0 {
		h.t.Fatalf("nota %v: exit %d\n%s%s", args, code, out, errs)
	}
	return out
}

func (h *harness) jsonList(args ...string) []jsonNote {
	var notes []jsonNote
	if err := json.Unmarshal([]byte(h.ok(append([]string{"list", "--json"}, args...)...)), &notes); err != nil {
		h.t.Fatal(err)
	}
	return notes
}

func TestArgs(t *testing.T) {
	p, err := parseArgs([]string{"check", "-t", "a", "indexes", "--in=2h", "--tag", "b", "--", "--literal"}, []flagSpec{
		{name: "in", value: true}, {name: "tag", short: "t", value: true, multi: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.args, " "); got != "check indexes --literal" {
		t.Fatalf("args %q", got)
	}
	if p.str("in") != "2h" || len(p.vals["tag"]) != 2 {
		t.Fatalf("flags %v", p.vals)
	}
	if _, err := parseArgs([]string{"--nope"}, nil); err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestAddListDoneFlow(t *testing.T) {
	h := newHarness(t)
	out := h.ok("add", "check MongoDB indexes #db", "--in", "2h")
	if !strings.Contains(out, "Added") || !strings.Contains(out, "in 2h") {
		t.Fatalf("add output: %q", out)
	}
	h.ok("add", "plain thought", "-t", "Ideas")
	h.ok("add", "tomorrow thing", "--at", "tomorrow 9am")

	notes := h.jsonList()
	if len(notes) != 3 {
		t.Fatalf("got %d notes", len(notes))
	}
	// reminders first, soonest first
	if notes[0].Text != "check MongoDB indexes #db" || notes[1].Text != "tomorrow thing" {
		t.Fatalf("order: %s, %s", notes[0].Text, notes[1].Text)
	}
	if notes[0].RemindAt == nil || !notes[0].RemindAt.Equal(fixedNow.Add(2*time.Hour)) {
		t.Fatalf("remind_at %v", notes[0].RemindAt)
	}
	if len(notes[0].Tags) != 1 || notes[0].Tags[0] != "db" {
		t.Fatalf("tags %v", notes[0].Tags)
	}

	if got := h.jsonList("--tag", "ideas"); len(got) != 1 || got[0].Text != "plain thought" {
		t.Fatalf("tag filter: %v", got)
	}
	if got := h.jsonList("--due"); len(got) != 2 {
		t.Fatalf("due filter: %d", len(got))
	}

	id := notes[2].ID
	h.ok("done", id[:4])
	if got := h.jsonList(); len(got) != 2 {
		t.Fatalf("done note still listed: %d", len(got))
	}
	if got := h.jsonList("--done"); len(got) != 1 || got[0].ID != id {
		t.Fatalf("--done: %v", got)
	}
	if got := h.jsonList("--all"); len(got) != 3 {
		t.Fatalf("--all: %d", len(got))
	}

	// search finds done notes too
	out = h.ok("search", "plain")
	if !strings.Contains(out, id) {
		t.Fatalf("search output %q", out)
	}

	h.ok("reopen", id)
	h.ok("rm", id)
	if got := h.jsonList("--all"); len(got) != 2 {
		t.Fatalf("after rm: %d", len(got))
	}
}

func TestAddFromStdin(t *testing.T) {
	h := newHarness(t)
	out, errs, code := h.run("NAME   READY\napi-1  0/1\n", "add", "-q")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	id := strings.TrimSpace(out)
	data, err := os.ReadFile(filepath.Join(h.dir, "notes", id+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "api-1  0/1") {
		t.Fatalf("file: %s", data)
	}
}

func TestErrors(t *testing.T) {
	h := newHarness(t)
	if _, _, code := h.run("", "add"); code != 2 {
		t.Fatalf("empty add exit %d", code)
	}
	if _, _, code := h.run("", "add", "x", "--in", "2h", "--at", "3pm"); code != 2 {
		t.Fatalf("in+at exit %d", code)
	}
	if _, errs, code := h.run("", "add", "x", "--at", "banana"); code != 1 || !strings.Contains(errs, "could not understand") {
		t.Fatalf("bad time: %d %q", code, errs)
	}
	if _, _, code := h.run("", "done", "zzzzzz"); code != 1 {
		t.Fatalf("missing id exit %d", code)
	}
	if _, errs, code := h.run("", "check", "this"); code != 2 || !strings.Contains(errs, "nota add") {
		t.Fatalf("unknown command: %d %q", code, errs)
	}
}

func TestSnoozeAndRemind(t *testing.T) {
	h := newHarness(t)
	id := strings.TrimSpace(h.ok("add", "-q", "x"))
	h.ok("snooze", id, "30m")
	n := h.jsonList()[0]
	if n.RemindAt == nil || !n.RemindAt.Equal(fixedNow.Add(30*time.Minute)) {
		t.Fatalf("snooze: %v", n.RemindAt)
	}
	h.ok("remind", id, "friday", "14:30")
	n = h.jsonList()[0]
	if n.RemindAt.Format("Mon 15:04") != "Fri 14:30" {
		t.Fatalf("remind: %v", n.RemindAt)
	}
	h.ok("remind", id, "off")
	if n = h.jsonList()[0]; n.RemindAt != nil {
		t.Fatalf("clear: %v", n.RemindAt)
	}
}

func TestHereUsesGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	h := newHarness(t)
	repo := filepath.Join(h.cwd, "api")
	os.MkdirAll(filepath.Join(repo, "sub"), 0o755)
	if out, err := exec.Command("git", "-C", repo, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Skip(string(out))
	}
	h.ok("add", "outside")
	h.cwd = filepath.Join(repo, "sub")
	h.ok("add", "inside repo")

	var notes []jsonNote
	json.Unmarshal([]byte(h.ok("here", "--json")), &notes)
	if len(notes) != 1 || notes[0].Text != "inside repo" || notes[0].GitRepo != "api" || notes[0].GitBranch != "main" {
		t.Fatalf("here: %+v", notes)
	}
}

func TestTableOutput(t *testing.T) {
	h := newHarness(t)
	h.ok("add", "a fairly long note about something", "-t", "ops", "--in", "90m")
	out := h.ok("list")
	if !strings.Contains(out, "in 1h30m") || !strings.Contains(out, "#ops") || strings.Contains(out, "\x1b[") {
		t.Fatalf("table: %q", out)
	}
	h.now = fixedNow.Add(3 * time.Hour)
	if out = h.ok("list"); !strings.Contains(out, "1h30m late") {
		t.Fatalf("late: %q", out)
	}
}
