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

func TestDashTextAndLimit(t *testing.T) {
	h := newHarness(t)
	h.ok("add", "-rf is dangerous")
	h.ok("add", "second", "--at=tomorrow 9am")
	if got := h.jsonList(); len(got) != 2 {
		t.Fatalf("got %d notes", len(got))
	}
	if got := h.jsonList("--limit", "1"); len(got) != 1 {
		t.Fatalf("limit: %d", len(got))
	}
	if _, _, code := h.run("", "list", "--limit", "2abc"); code != 2 {
		t.Fatalf("junk limit exit %d", code)
	}
}

func TestShowAndPath(t *testing.T) {
	h := newHarness(t)
	id := strings.TrimSpace(h.ok("add", "-q", "inspect the node pool", "-t", "k8s", "--in", "1h"))
	out := h.ok("show", id)
	for _, want := range []string{id, "inspect the node pool", "k8s"} {
		if !strings.Contains(out, want) {
			t.Errorf("show missing %q:\n%s", want, out)
		}
	}
	if got := strings.TrimSpace(h.ok("path")); got != h.dir {
		t.Errorf("path = %q, want %q", got, h.dir)
	}
}

func TestRemindReopensDoneNote(t *testing.T) {
	h := newHarness(t)
	id := strings.TrimSpace(h.ok("add", "-q", "revisit"))
	h.ok("done", id)
	h.ok("snooze", id, "1h")
	got := h.jsonList()
	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("snoozed note should be open again: %+v", got)
	}
}

func TestRmMultipleWithOneMissing(t *testing.T) {
	h := newHarness(t)
	a := strings.TrimSpace(h.ok("add", "-q", "a"))
	_, errs, code := h.run("", "rm", a, "nope99")
	if code != 1 || !strings.Contains(errs, "nope99") {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	if got := h.jsonList("--all"); len(got) != 0 {
		t.Fatalf("existing note should still be removed: %d left", len(got))
	}
}

func TestEditRunsEditorAndValidates(t *testing.T) {
	h := newHarness(t)
	id := strings.TrimSpace(h.ok("add", "-q", "original"))

	// an "editor" that appends a line, with arguments, to prove $EDITOR is run via sh
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `sh -c 'echo "added by editor" >> "$0"'`)
	h.ok("edit", id)
	out := h.ok("show", id)
	if !strings.Contains(out, "added by editor") {
		t.Fatalf("edit not applied:\n%s", out)
	}

	// an editor that corrupts the front matter must produce an error, not silent loss
	t.Setenv("EDITOR", `sh -c 'printf "%s\n" "---" "status: maybe" "---" "x" > "$0"'`)
	if _, errs, code := h.run("", "edit", id); code == 0 || !strings.Contains(errs, "cannot be parsed") {
		t.Fatalf("expected parse error, code=%d stderr=%q", code, errs)
	}
}

func TestSearchRequiresAllWords(t *testing.T) {
	h := newHarness(t)
	h.ok("add", "rotate the staging database password")
	h.ok("add", "database backup failed")
	out := h.ok("search", "database", "staging")
	if !strings.Contains(out, "staging") || strings.Contains(out, "backup") {
		t.Fatalf("search should AND the words:\n%s", out)
	}
}

func TestQuotedSentenceIsAdd(t *testing.T) {
	h := newHarness(t)
	h.ok("hello there", "--in", "2h", "-t", "ideas")
	h.ok("another one")
	h.ok("-t", "ideas", "--in=1h", "flags come first")
	h.ok("--in", "1h", "hello world")
	got := h.jsonList()
	if len(got) != 4 {
		t.Fatalf("got %d notes", len(got))
	}
	if got := h.jsonList("--tag", "ideas"); len(got) != 2 || got[0].RemindAt == nil {
		t.Fatalf("flags must be honoured: %+v", got)
	}

	// a mistyped command or lone word is still an error, never a note
	for _, args := range [][]string{{"snoze", "3k"}, {"hello"}, {"-x"}, {"--done"}} {
		if _, _, code := h.run("", args...); code != 2 {
			t.Errorf("nota %v: exit %d, want 2", args, code)
		}
	}
	if n := len(h.jsonList("--all")); n != 4 {
		t.Fatalf("typos created notes: %d", n)
	}
}

func TestAppend(t *testing.T) {
	h := newHarness(t)
	id := strings.TrimSpace(h.ok("add", "deploy checklist #ops", "-q"))

	if out := h.ok("append", id[:4], "drain node-3 first #k8s"); !strings.Contains(out, "Appended to "+id) {
		t.Fatalf("append output: %q", out)
	}
	if _, _, code := h.run("kubectl get pods\npod-a Running\n", "append", id); code != 0 {
		t.Fatal("append from stdin failed")
	}
	h.ok("append", id, "--", "-rf the old cache")

	n := h.jsonList()[0]
	want := "deploy checklist #ops\ndrain node-3 first #k8s\nkubectl get pods\npod-a Running\n-rf the old cache"
	if n.Text != want {
		t.Fatalf("body:\n%q\nwant:\n%q", n.Text, want)
	}
	if strings.Join(n.Tags, ",") != "k8s,ops" {
		t.Fatalf("tags from appended text must be merged: %v", n.Tags)
	}

	h.ok("done", id)
	h.ok("append", id, "follow up done")
	if got := h.jsonList("--done"); len(got) != 1 {
		t.Fatalf("append must not change the status: %+v", got)
	}

	for _, args := range [][]string{{"append"}, {"append", id}, {"append", "zzzz", "text"}} {
		if _, _, code := h.run("", args...); code == 0 {
			t.Fatalf("nota %v: want an error", args)
		}
	}
}

func TestListNumbers(t *testing.T) {
	h := newHarness(t)
	h.ok("add", "first")
	h.now = h.now.Add(time.Minute)
	h.ok("add", "second")
	h.now = h.now.Add(time.Minute)
	h.ok("add", "third")

	// newest first: 1=third 2=second 3=first
	list := h.jsonList()
	if list[0].Text != "third" || list[2].Text != "first" {
		t.Fatalf("unexpected order: %v", list)
	}
	out := h.ok("list")
	if !strings.HasPrefix(strings.TrimSpace(out), "1") || !strings.Contains(out, list[0].ID) {
		t.Fatalf("list should be numbered: %q", out)
	}

	// several numbers in one command refer to the list as it was before
	h.ok("rm", "2", "3")
	left := h.jsonList()
	if len(left) != 1 || left[0].Text != "third" {
		t.Fatalf("after rm 2 3: %v", left)
	}

	// ids still work; an unknown reference fails (ids never contain 0)
	if _, _, code := h.run("", "show", "0"); code == 0 {
		t.Fatal("show 0 should fail")
	}
	h.ok("done", left[0].ID)
	if got := h.jsonList(); len(got) != 0 {
		t.Fatalf("done by id: %v", got)
	}
}
