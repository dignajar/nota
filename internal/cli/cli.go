// Package cli implements the nota command line.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dignajar/nota/internal/store"
)

// Version is set at build time with -ldflags "-X github.com/dignajar/nota/internal/cli.Version=v1.2.3".
var Version = "dev"

// App holds IO and shared state so commands are testable.
type App struct {
	Out, Err   io.Writer
	In         io.Reader
	StdinIsTTY bool
	Now        func() time.Time
	Dir        string // data dir; resolved from env when empty
	Cwd        string // working dir; os.Getwd when empty

	color palette
	st    *store.Store
}

type command struct {
	name    string
	aliases []string
	usage   string
	summary string
	flags   []flagSpec
	run     func(a *App, p *parsed) error
}

var commands []*command

func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
		for _, al := range c.aliases {
			if al == name {
				return c
			}
		}
	}
	return nil
}

// Main runs the CLI with real process IO and returns the exit code.
func Main(args []string) int {
	fi, _ := os.Stdin.Stat()
	outFi, _ := os.Stdout.Stat()
	a := &App{
		Out:        os.Stdout,
		Err:        os.Stderr,
		In:         os.Stdin,
		StdinIsTTY: fi != nil && fi.Mode()&os.ModeCharDevice != 0,
		Now:        time.Now,
	}
	a.color.on = outFi != nil && outFi.Mode()&os.ModeCharDevice != 0 &&
		os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return a.Run(args)
}

// Run executes one invocation. args excludes the program name.
func (a *App) Run(args []string) int {
	if a.Now == nil {
		a.Now = time.Now
	}
	if len(args) == 0 {
		args = []string{"list"}
	}
	name := args[0]
	switch name {
	case "-h", "--help", "help":
		if len(args) > 1 {
			if c := lookup(args[1]); c != nil {
				a.commandHelp(c)
				return 0
			}
		}
		a.help()
		return 0
	case "-v", "--version", "version":
		fmt.Fprintln(a.Out, "nota", Version)
		return 0
	}
	c := lookup(name)
	if c == nil && (isSentence(name) || isAddFlag(name) || isBareNote(args)) {
		// nota "hello there", nota --in 1h "hello" and nota hello are
		// shorthand for nota add
		c, args = lookup("add"), append([]string{"add"}, args...)
	}
	if c == nil {
		fmt.Fprintf(a.Err, "nota: unknown command %q\n", name)
		fmt.Fprintf(a.Err, "Did you mean: nota add %q ?  (see nota help)\n", strings.Join(args, " "))
		return 2
	}
	for _, x := range args[1:] {
		if x == "-h" || x == "--help" {
			a.commandHelp(c)
			return 0
		}
		if x == "--" {
			break
		}
	}
	p, err := parseArgs(args[1:], c.flags)
	if err != nil {
		fmt.Fprintf(a.Err, "nota %s: %v\n", c.name, err)
		fmt.Fprintf(a.Err, "usage: nota %s\n", c.usage)
		return 2
	}
	if err := c.run(a, p); err != nil {
		var u usageError
		if errors.As(err, &u) {
			fmt.Fprintf(a.Err, "nota %s: %v\nusage: nota %s\n", c.name, err, c.usage)
			return 2
		}
		fmt.Fprintf(a.Err, "nota %s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// isSentence reports whether s is free text, that is it contains whitespace,
// which a command name never does.
func isSentence(s string) bool {
	return !strings.HasPrefix(s, "-") && strings.ContainsAny(s, " \t\n")
}

// isAddFlag reports whether arg is one of the add command's flags, so
// nota --in 1h "hello" works the same as nota add --in 1h "hello".
func isAddFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return false
	}
	name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	long := strings.HasPrefix(arg, "--")
	for _, f := range lookup("add").flags {
		if (long && f.name == name) || (!long && f.short != "" && f.short == name) {
			return true
		}
	}
	return false
}

// isBareNote reports whether args is a single unknown word, optionally
// followed only by add flags (nota hh, nota hh --in 1h), that is not close to
// any command name. Close words (nota snoze 3k, nota dnoe) stay errors so a
// typo never silently becomes a note, and extra words (nota check this) stay
// errors because quoting them is the clear way to write a sentence.
func isBareNote(args []string) bool {
	name := args[0]
	if name == "" || strings.HasPrefix(name, "-") || nearCommand(name) {
		return false
	}
	p, err := parseArgs(args[1:], lookup("add").flags)
	return err == nil && len(p.args) == 0
}

// nearCommand reports whether word is within a few edits of a command name or
// alias. Short words must match exactly so one or two letter notes work.
func nearCommand(word string) bool {
	word = strings.ToLower(word)
	max := 2
	switch n := len([]rune(word)); {
	case n <= 2:
		max = 0
	case n <= 5:
		max = 1
	}
	for _, c := range commands {
		for _, name := range append([]string{c.name}, c.aliases...) {
			if editDistance(word, name) <= max {
				return true
			}
		}
	}
	return false
}

// editDistance is the optimal string alignment distance: insert, delete,
// substitute and swap of adjacent letters each cost one.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

func usagef(format string, args ...any) error { return usageError{fmt.Sprintf(format, args...)} }

func (a *App) now() time.Time { return a.Now() }

func (a *App) store() (*store.Store, error) {
	if a.st != nil {
		return a.st, nil
	}
	dir := a.Dir
	if dir == "" {
		var err error
		if dir, err = store.DefaultDir(); err != nil {
			return nil, err
		}
	}
	s, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	a.st = s
	return s, nil
}

func (a *App) cwd() string {
	if a.Cwd != "" {
		return a.Cwd
	}
	d, _ := os.Getwd()
	return d
}

func (a *App) warn(format string, args ...any) {
	fmt.Fprintf(a.Err, "warning: "+format+"\n", args...)
}

func (a *App) help() {
	w := a.Out
	fmt.Fprintf(w, "nota %s: capture notes and reminders without leaving the terminal\n\n", Version)
	fmt.Fprintln(w, "Usage:")
	width := 0
	for _, c := range commands {
		if l := len(c.name); l > width {
			width = l
		}
	}
	for _, c := range commands {
		al := ""
		if len(c.aliases) > 0 {
			al = a.color.dim(" (" + strings.Join(c.aliases, ", ") + ")")
		}
		fmt.Fprintf(w, "  %-*s  %s%s\n", width, c.name, c.summary, al)
	}
	fmt.Fprint(w, `
Examples:
  nota add "check the ingress-nginx rollout in prod #k8s"
  nota add "try the new rollout command" --in 2h
  nota add "look into flaky test" --at "tomorrow 9am"
  nota "quick thought" --in 1h              # same as nota add "quick thought"
  nota --in 1h "quick thought"              # flags first work too
  nota groceries --in 2h                    # one word works too (unless it looks like a command)
  kubectl get pods -A | nota add            # note from stdin
  nota                                      # list open notes
  nota here                                 # notes for this repo
  nota done k3f

Times: 2h, 1h30m, 3 days, tomorrow, tomorrow 9am, tonight, friday 14:30,
       next monday, eod, 2026-10-20 14:00

Environment:
  NOTA_HOME        data dir (default $XDG_DATA_HOME/nota or ~/.local/share/nota)
  NOTA_NOTIFY_CMD  custom notification command; gets $NOTA_TITLE and $NOTA_BODY
  EDITOR, VISUAL   used by nota edit
  NO_COLOR         disable colors

Run 'nota help <command>' for command flags.
`)
}

func (a *App) commandHelp(c *command) {
	fmt.Fprintf(a.Out, "nota %s\n\n%s\n", c.usage, c.summary)
	if len(c.aliases) > 0 {
		fmt.Fprintf(a.Out, "Aliases: %s\n", strings.Join(c.aliases, ", "))
	}
	if len(c.flags) == 0 {
		return
	}
	fmt.Fprintln(a.Out, "\nFlags:")
	specs := append([]flagSpec(nil), c.flags...)
	sort.Slice(specs, func(i, j int) bool { return specs[i].name < specs[j].name })
	for _, f := range specs {
		left := "    --" + f.name
		if f.short != "" {
			left = "-" + f.short + ", --" + f.name
		}
		if f.value {
			left += " " + f.arg
		}
		fmt.Fprintf(a.Out, "  %-26s %s\n", left, f.help)
	}
}
