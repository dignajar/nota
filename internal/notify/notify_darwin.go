//go:build darwin

package notify

import (
	"os/exec"
	"strings"
)

type darwin struct{}

func platform() Notifier { return darwin{} }

// Send prefers terminal-notifier (brew install terminal-notifier) when present,
// since it shows a proper app identity, and falls back to AppleScript.
func (darwin) Send(title, body string) error {
	if p, err := exec.LookPath("terminal-notifier"); err == nil {
		return run(p, "-title", title, "-message", body, "-group", "nota", "-sound", "default")
	}
	script := `display notification "` + appleEscape(body) + `" with title "` + appleEscape(title) + `" sound name "default"`
	return run("/usr/bin/osascript", "-e", script)
}

func (darwin) Describe() string {
	if p, err := exec.LookPath("terminal-notifier"); err == nil {
		return "terminal-notifier (" + p + ")"
	}
	return "osascript (install terminal-notifier for nicer notifications)"
}

func appleEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
