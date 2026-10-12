//go:build darwin

package notify

import "strings"

type darwin struct{}

func platform() Notifier { return darwin{} }

// Send uses AppleScript, which ships with every Mac.
func (darwin) Send(title, body string) error {
	script := `display notification "` + appleEscape(body) + `" with title "` + appleEscape(title) + `" sound name "default"`
	return run("/usr/bin/osascript", "-e", script)
}

func (darwin) Describe() string { return "osascript (built into macOS)" }

func appleEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
