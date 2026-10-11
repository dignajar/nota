//go:build linux

package notify

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

type linux struct{}

func platform() Notifier { return linux{} }

// Send uses notify-send (libnotify) and falls back to calling the
// freedesktop notification service over D-Bus with gdbus.
func (linux) Send(title, body string) error {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		// systemd user services usually have it; set the default path if not
		if _, err := os.Stat(defaultBusPath()); err == nil {
			os.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+defaultBusPath())
		}
	}
	if p, err := exec.LookPath("notify-send"); err == nil {
		return run(p, "--app-name=nota", "--urgency=normal", title, body)
	}
	if p, err := exec.LookPath("gdbus"); err == nil {
		return run(p, "call", "--session",
			"--dest", "org.freedesktop.Notifications",
			"--object-path", "/org/freedesktop/Notifications",
			"--method", "org.freedesktop.Notifications.Notify",
			"nota", "0", "", title, body, "[]", "{}", "10000")
	}
	return errors.New("neither notify-send nor gdbus found (install libnotify-bin or libnotify), or set NOTA_NOTIFY_CMD")
}

func (linux) Describe() string {
	if p, err := exec.LookPath("notify-send"); err == nil {
		return "notify-send (" + p + ")"
	}
	if p, err := exec.LookPath("gdbus"); err == nil {
		return "gdbus (" + p + ")"
	}
	return "none found: install libnotify-bin (Debian/Ubuntu) or libnotify (Fedora/Arch)"
}

func defaultBusPath() string {
	return "/run/user/" + strconv.Itoa(os.Getuid()) + "/bus"
}
