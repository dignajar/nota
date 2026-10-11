//go:build linux

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const unitName = "nota.service"

type systemd struct{}

func platform() Manager { return systemd{} }

func unitPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user", unitName)
}

// Unit renders the systemd user unit.
func Unit(binary, dataDir string) string {
	return fmt.Sprintf(`[Unit]
Description=nota reminder daemon
Documentation=https://github.com/dignajar/nota
After=graphical-session.target

[Service]
Type=simple
ExecStart=%s daemon
Environment=NOTA_HOME=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, systemdQuote(binary), systemdQuote(dataDir))
}

func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func (systemd) Install(binary, dataDir string) (string, error) {
	path := unitPath()
	if err := writeFile(path, Unit(binary, dataDir)); err != nil {
		return "", err
	}
	// Make the desktop session bus visible to user services on setups where
	// it is not exported automatically. Harmless if it fails.
	vars := []string{}
	for _, v := range []string{"DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"} {
		if os.Getenv(v) != "" {
			vars = append(vars, v)
		}
	}
	if len(vars) > 0 {
		run("systemctl", append([]string{"--user", "import-environment"}, vars...)...)
	}
	if _, err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return path, err
	}
	if _, err := run("systemctl", "--user", "enable", "--now", unitName); err != nil {
		return path, err
	}
	// pick up a new binary or data dir if it was already running
	_, err := run("systemctl", "--user", "restart", unitName)
	return path, err
}

func (systemd) Uninstall() error {
	run("systemctl", "--user", "disable", "--now", unitName)
	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, err := run("systemctl", "--user", "daemon-reload")
	return err
}

func (systemd) Status() string {
	if _, err := os.Stat(unitPath()); err != nil {
		return "not installed (run: nota install-service)"
	}
	out, _ := run("systemctl", "--user", "is-active", unitName)
	if out == "" {
		out = "unknown"
	}
	return out + " (" + unitPath() + "; logs: journalctl --user -u nota)"
}
