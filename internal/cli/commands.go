package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dignajar/nota/internal/gitctx"
	"github.com/dignajar/nota/internal/notify"
	"github.com/dignajar/nota/internal/scheduler"
	"github.com/dignajar/nota/internal/service"
	"github.com/dignajar/nota/internal/store"
	"github.com/dignajar/nota/internal/timeparse"
)

var listFlags = []flagSpec{
	{name: "all", short: "a", help: "include done notes"},
	{name: "done", help: "only done notes"},
	{name: "due", short: "d", help: "only notes with a pending or overdue reminder"},
	{name: "tag", short: "t", value: true, multi: true, arg: "TAG", help: "filter by tag (repeatable, all must match)"},
	{name: "here", help: "only notes captured in this repo or directory"},
	{name: "limit", short: "n", value: true, arg: "N", help: "show at most N notes"},
	{name: "json", help: "machine readable output"},
}

func init() {
	commands = []*command{
		{
			name: "add", aliases: []string{"a"},
			usage:   `add "text" [--in 2h | --at "tomorrow 9am"] [-t tag]`,
			summary: "capture a note, optionally with a reminder (reads stdin when no text)",
			flags: []flagSpec{
				{name: "in", short: "i", value: true, arg: "DURATION", help: "remind after a duration: 30m, 2h, 1d"},
				{name: "at", value: true, arg: "TIME", help: `remind at a time: "tomorrow 9am", "fri 14:30"`},
				{name: "tag", short: "t", value: true, multi: true, arg: "TAG", help: "add a tag (repeatable); #tags in the text also work"},
				{name: "no-context", help: "do not record directory and git repo"},
				{name: "quiet", short: "q", help: "print only the new id"},
			},
			run: cmdAdd,
		},
		{name: "list", aliases: []string{"ls", "l"}, usage: "list [flags]", summary: "list open notes (default command)", flags: listFlags, run: cmdList},
		{name: "here", usage: "here [flags]", summary: "list notes captured in the current repo or directory", flags: listFlags,
			run: func(a *App, p *parsed) error { p.bools["here"] = true; return cmdList(a, p) }},
		{name: "search", aliases: []string{"s", "find"}, usage: "search <words...> [flags]", summary: "find notes containing all words", flags: listFlags, run: cmdSearch},
		{name: "show", aliases: []string{"cat"}, usage: "show <id>", summary: "show one note with its details", run: cmdShow},
		{name: "done", aliases: []string{"d"}, usage: "done <id>...", summary: "mark notes as done", run: setStatus(store.StatusDone)},
		{name: "reopen", usage: "reopen <id>...", summary: "mark notes as open again", run: setStatus(store.StatusOpen)},
		{name: "rm", aliases: []string{"del", "delete"}, usage: "rm <id>...", summary: "delete notes", run: cmdRm},
		{name: "edit", aliases: []string{"e"}, usage: "edit <id>", summary: "open a note in $EDITOR", run: cmdEdit},
		{name: "snooze", usage: "snooze <id> [duration]", summary: "remind again later (default 1h)", run: cmdSnooze},
		{name: "remind", usage: `remind <id> <time|off>`, summary: "set or clear the reminder of an existing note", run: cmdRemind},
		{name: "daemon", usage: "daemon [--interval 5s]", summary: "run the reminder scheduler in the foreground",
			flags: []flagSpec{{name: "interval", value: true, arg: "DURATION", help: "max time between checks (default 5s)"}},
			run:   cmdDaemon},
		{name: "install-service", usage: "install-service", summary: "run the daemon in the background (systemd or launchd)", run: cmdInstall},
		{name: "uninstall-service", usage: "uninstall-service", summary: "remove the background service", run: cmdUninstall},
		{name: "doctor", usage: "doctor [--no-test]", summary: "check setup and send a test notification",
			flags: []flagSpec{{name: "no-test", help: "skip the test notification"}}, run: cmdDoctor},
		{name: "path", usage: "path", summary: "print the data directory", run: cmdPath},
	}
}

// ---- add ----

func cmdAdd(a *App, p *parsed) error {
	text := strings.TrimSpace(strings.Join(p.args, " "))
	if text == "" && !a.StdinIsTTY && a.In != nil {
		b, err := io.ReadAll(io.LimitReader(a.In, 1<<20))
		if err != nil {
			return err
		}
		text = strings.TrimSpace(string(b))
	}
	if text == "" {
		return usagef("nothing to add")
	}
	if p.has("in") && p.has("at") {
		return usagef("use either --in or --at, not both")
	}

	n := &store.Note{Body: text, Created: a.now()}
	switch {
	case p.has("in"):
		d, err := timeparse.Duration(p.str("in"))
		if err != nil {
			return fmt.Errorf("--in: %w (examples: 30m, 2h, 1h30m, 2d)", err)
		}
		n.RemindAt = a.now().Add(d).Truncate(time.Second)
	case p.has("at"):
		t, err := timeparse.Parse(p.str("at"), a.now())
		if err != nil {
			return fmt.Errorf("--at: %w", err)
		}
		n.RemindAt = t.Truncate(time.Second)
	}
	n.Tags = store.NormalizeTags(append(store.ExtractTags(text), p.vals["tag"]...))
	if !p.bools["no-context"] {
		c := gitctx.Detect(a.cwd())
		n.Cwd, n.GitRoot, n.GitRepo, n.GitBranch = c.Cwd, c.Root, c.Repo, c.Branch
	}

	s, err := a.store()
	if err != nil {
		return err
	}
	if err := s.Create(n); err != nil {
		return err
	}
	if p.bools["quiet"] {
		fmt.Fprintln(a.Out, n.ID)
		return nil
	}
	msg := "Added " + a.color.cyan(n.ID)
	if n.HasReminder() {
		msg += "  reminder " + a.color.yellow(fullTime(n.RemindAt, a.now()))
	}
	fmt.Fprintln(a.Out, msg)
	return nil
}

// ---- list / here / search ----

func cmdList(a *App, p *parsed) error {
	if len(p.args) > 0 {
		return usagef("unexpected argument %q (did you mean nota add or nota search?)", p.args[0])
	}
	return a.listWhere(p, nil)
}

func cmdSearch(a *App, p *parsed) error {
	words := strings.Fields(strings.ToLower(strings.Join(p.args, " ")))
	if len(words) == 0 {
		return usagef("missing search words")
	}
	if !p.has("done") {
		p.bools["all"] = true // searching should find done notes too
	}
	return a.listWhere(p, func(n *store.Note) bool {
		hay := strings.ToLower(n.Body + " " + strings.Join(n.Tags, " ") + " " + n.GitRepo + " " + n.GitBranch)
		for _, w := range words {
			if !strings.Contains(hay, strings.TrimPrefix(w, "#")) {
				return false
			}
		}
		return true
	})
}

func (a *App) listWhere(p *parsed, match func(*store.Note) bool) error {
	s, err := a.store()
	if err != nil {
		return err
	}
	notes, warns := s.LoadAll()
	for _, w := range warns {
		a.warn("skipped %v", w)
	}
	limit := 0
	if v := p.str("limit"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &limit); err != nil || limit < 0 {
			return usagef("--limit needs a positive number")
		}
	}
	var here gitctx.Context
	if p.bools["here"] {
		here = gitctx.Detect(a.cwd())
	}

	var out []*store.Note
	for _, n := range notes {
		switch {
		case p.bools["done"]:
			if n.IsOpen() {
				continue
			}
		case !p.bools["all"]:
			if !n.IsOpen() {
				continue
			}
		}
		if p.bools["due"] && !(n.IsOpen() && n.HasReminder()) {
			continue
		}
		if !hasAllTags(n, p.vals["tag"]) {
			continue
		}
		if p.bools["here"] && !sameContext(n, here) {
			continue
		}
		if match != nil && !match(n) {
			continue
		}
		out = append(out, n)
	}
	store.Sort(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	if p.bools["json"] {
		return a.printJSON(out)
	}
	if len(out) == 0 {
		fmt.Fprintln(a.Err, a.color.dim("no notes"))
		return nil
	}
	a.printTable(out)
	return nil
}

func hasAllTags(n *store.Note, tags []string) bool {
	for _, t := range tags {
		if !n.HasTag(t) {
			return false
		}
	}
	return true
}

func sameContext(n *store.Note, c gitctx.Context) bool {
	if c.Root != "" {
		return n.GitRoot == c.Root
	}
	return n.Cwd == c.Cwd
}

type jsonNote struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Status    string     `json:"status"`
	Created   time.Time  `json:"created"`
	RemindAt  *time.Time `json:"remind_at,omitempty"`
	Notified  bool       `json:"notified"`
	Tags      []string   `json:"tags"`
	Cwd       string     `json:"cwd,omitempty"`
	GitRepo   string     `json:"git_repo,omitempty"`
	GitBranch string     `json:"git_branch,omitempty"`
	Path      string     `json:"path"`
}

func (a *App) printJSON(notes []*store.Note) error {
	out := make([]jsonNote, 0, len(notes))
	for _, n := range notes {
		j := jsonNote{ID: n.ID, Text: n.Body, Status: string(n.Status), Created: n.Created, Notified: n.Notified,
			Tags: n.Tags, Cwd: n.Cwd, GitRepo: n.GitRepo, GitBranch: n.GitBranch, Path: n.Path}
		if j.Tags == nil {
			j.Tags = []string{}
		}
		if n.HasReminder() {
			t := n.RemindAt
			j.RemindAt = &t
		}
		out = append(out, j)
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ---- single note commands ----

func (a *App) find(id string) (*store.Note, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	return s.Find(id)
}

func cmdShow(a *App, p *parsed) error {
	if len(p.args) != 1 {
		return usagef("expected one id")
	}
	n, err := a.find(p.args[0])
	if err != nil {
		return err
	}
	a.printNote(n)
	return nil
}

func setStatus(st store.Status) func(*App, *parsed) error {
	return func(a *App, p *parsed) error {
		if len(p.args) == 0 {
			return usagef("expected at least one id")
		}
		s, err := a.store()
		if err != nil {
			return err
		}
		var failed error
		for _, id := range p.args {
			n, err := s.Find(id)
			if err != nil {
				fmt.Fprintf(a.Err, "%s: %v\n", id, err)
				failed = errors.New("some notes were not updated")
				continue
			}
			if _, err := s.Update(n.ID, func(n *store.Note) { n.Status = st }); err != nil {
				return err
			}
			verb := "Done"
			if st == store.StatusOpen {
				verb = "Reopened"
			}
			fmt.Fprintf(a.Out, "%s %s  %s\n", verb, a.color.cyan(n.ID), truncate(n.Title(), 60))
		}
		return failed
	}
}

func cmdRm(a *App, p *parsed) error {
	if len(p.args) == 0 {
		return usagef("expected at least one id")
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	var failed error
	for _, id := range p.args {
		n, err := s.Find(id)
		if err != nil {
			fmt.Fprintf(a.Err, "%s: %v\n", id, err)
			failed = errors.New("some notes were not removed")
			continue
		}
		if err := s.Delete(n.ID); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Removed %s  %s\n", a.color.cyan(n.ID), truncate(n.Title(), 60))
	}
	return failed
}

func cmdEdit(a *App, p *parsed) error {
	if len(p.args) != 1 {
		return usagef("expected one id")
	}
	n, err := a.find(p.args[0])
	if err != nil {
		return err
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// $EDITOR may contain arguments, e.g. "code --wait"
	cmd := exec.Command("/bin/sh", "-c", editor+` "$1"`, "nota-edit", n.Path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor: %w", err)
	}
	data, err := os.ReadFile(n.Path)
	if err != nil {
		return err
	}
	if _, err := store.Unmarshal(data); err != nil {
		return fmt.Errorf("the file was saved but cannot be parsed, fix it with `nota edit %s`: %w", n.ID, err)
	}
	return nil
}

func cmdSnooze(a *App, p *parsed) error {
	if len(p.args) < 1 {
		return usagef("expected an id")
	}
	d := time.Hour
	if len(p.args) > 1 {
		var err error
		if d, err = timeparse.Duration(strings.Join(p.args[1:], " ")); err != nil {
			return err
		}
	}
	return a.setReminder(p.args[0], a.now().Add(d))
}

func cmdRemind(a *App, p *parsed) error {
	if len(p.args) < 2 {
		return usagef("expected an id and a time")
	}
	when := strings.Join(p.args[1:], " ")
	switch strings.ToLower(when) {
	case "off", "none", "clear":
		return a.setReminder(p.args[0], time.Time{})
	}
	t, err := timeparse.Parse(when, a.now())
	if err != nil {
		return err
	}
	return a.setReminder(p.args[0], t)
}

func (a *App) setReminder(id string, t time.Time) error {
	n, err := a.find(id)
	if err != nil {
		return err
	}
	n, err = a.st.Update(n.ID, func(n *store.Note) {
		n.RemindAt = t.Truncate(time.Second)
		n.Notified = false
		if !t.IsZero() {
			n.Status = store.StatusOpen
		}
	})
	if err != nil {
		return err
	}
	if t.IsZero() {
		fmt.Fprintf(a.Out, "Cleared reminder on %s\n", a.color.cyan(n.ID))
		return nil
	}
	fmt.Fprintf(a.Out, "Reminder on %s  %s\n", a.color.cyan(n.ID), a.color.yellow(fullTime(n.RemindAt, a.now())))
	return nil
}

// ---- daemon / service ----

func cmdDaemon(a *App, p *parsed) error {
	interval := 5 * time.Second
	if v := p.str("interval"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 100*time.Millisecond {
			return usagef("--interval must be a duration of at least 100ms")
		}
		interval = d
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sch := &scheduler.Scheduler{
		Store:    s,
		Notifier: notify.New(),
		Interval: interval,
		Now:      a.Now,
		Log:      log.New(a.Err, "nota: ", log.LstdFlags),
	}
	return sch.Run(ctx)
}

func cmdInstall(a *App, p *parsed) error {
	bin, err := service.Binary()
	if err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	path, err := service.New().Install(bin, s.Dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Installed %s\nDaemon: %s\n", path, service.New().Status())
	fmt.Fprintln(a.Out, "Try it: nota add \"hello from nota\" --in 1m")
	return nil
}

func cmdUninstall(a *App, p *parsed) error {
	if err := service.New().Uninstall(); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Service removed. Your notes are untouched.")
	return nil
}

func cmdPath(a *App, p *parsed) error {
	s, err := a.store()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Out, s.Dir)
	return nil
}

func cmdDoctor(a *App, p *parsed) error {
	ok := a.color.green("ok")
	bad := a.color.red("!!")
	problems := 0
	check := func(good bool, label, detail string) {
		mark := ok
		if !good {
			mark = bad
			problems++
		}
		fmt.Fprintf(a.Out, "[%s] %-14s %s\n", mark, label, detail)
	}

	s, err := a.store()
	if err != nil {
		check(false, "data dir", err.Error())
		return errors.New("data dir unusable")
	}
	probe := filepath.Join(s.NotesDir(), ".doctor-probe")
	werr := os.WriteFile(probe, []byte("ok"), 0o600)
	os.Remove(probe)
	check(werr == nil, "data dir", s.Dir)

	notes, warns := s.LoadAll()
	open, pending := 0, 0
	for _, n := range notes {
		if n.IsOpen() {
			open++
			if n.HasReminder() && !n.Notified {
				pending++
			}
		}
	}
	check(len(warns) == 0, "notes", fmt.Sprintf("%d total, %d open, %d pending reminders, %d unreadable", len(notes), open, pending, len(warns)))
	for _, w := range warns {
		fmt.Fprintf(a.Out, "      %v\n", w)
	}

	n := notify.New()
	desc := n.Describe()
	check(!strings.HasPrefix(desc, "none"), "notifier", desc)

	st := service.New().Status()
	check(!strings.HasPrefix(st, "not installed") && !strings.HasPrefix(st, "inactive") && !strings.HasPrefix(st, "failed"), "service", st)

	if _, err := exec.LookPath("git"); err != nil {
		check(false, "git", "not found: notes will not record repo and branch")
	} else {
		check(true, "git", "found")
	}

	if !p.bools["no-test"] {
		err := n.Send("nota", "Test notification from nota doctor")
		detail := "sent, you should see it now"
		if err != nil {
			detail = err.Error()
		}
		check(err == nil, "test", detail)
	}
	if problems > 0 {
		return fmt.Errorf("%d problem(s) found", problems)
	}
	return nil
}
