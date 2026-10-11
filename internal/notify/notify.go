// Package notify sends desktop notifications on macOS and Linux.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Notifier sends one notification.
type Notifier interface {
	Send(title, body string) error
	// Describe says which backend will be used, for `nota doctor`.
	Describe() string
}

// New picks the backend. If NOTA_NOTIFY_CMD is set it wins on every OS: the
// command runs via sh with NOTA_TITLE and NOTA_BODY in the environment, which
// lets headless machines forward reminders (ntfy, Slack webhook, etc).
func New() Notifier {
	if cmd := strings.TrimSpace(os.Getenv("NOTA_NOTIFY_CMD")); cmd != "" {
		return commandNotifier{cmd: cmd}
	}
	return platform()
}

type commandNotifier struct{ cmd string }

func (c commandNotifier) Send(title, body string) error {
	cmd := exec.Command("/bin/sh", "-c", c.cmd)
	cmd.Env = append(os.Environ(), "NOTA_TITLE="+title, "NOTA_BODY="+body)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("NOTA_NOTIFY_CMD failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c commandNotifier) Describe() string { return "custom command (NOTA_NOTIFY_CMD): " + c.cmd }

type unsupported struct{}

func (unsupported) Send(string, string) error {
	return fmt.Errorf("notifications are not supported on %s; set NOTA_NOTIFY_CMD", runtime.GOOS)
}
func (unsupported) Describe() string { return "none (unsupported OS)" }

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%s: %v: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
}
