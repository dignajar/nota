//go:build darwin

package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const label = "dev.nota.daemon"

type launchd struct{}

func platform() Manager { return launchd{} }

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Plist renders the launchd agent definition.
func Plist(binary, dataDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>NOTA_HOME</key>
		<string>%s</string>
		<key>PATH</key>
		<string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, label, esc(binary), esc(dataDir),
		esc(filepath.Join(dataDir, "daemon.log")), esc(filepath.Join(dataDir, "daemon.log")))
}

func (launchd) Install(binary, dataDir string) (string, error) {
	path := plistPath()
	if err := writeFile(path, Plist(binary, dataDir)); err != nil {
		return "", err
	}
	run("launchctl", "bootout", domain(), path) // ignore: not loaded yet
	if _, err := run("launchctl", "bootstrap", domain(), path); err != nil {
		// older macOS versions
		if _, err2 := run("launchctl", "load", "-w", path); err2 != nil {
			return path, err
		}
	}
	return path, nil
}

func (launchd) Uninstall() error {
	path := plistPath()
	if _, err := run("launchctl", "bootout", domain(), path); err != nil {
		run("launchctl", "unload", "-w", path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (launchd) Status() string {
	if _, err := os.Stat(plistPath()); err != nil {
		return "not installed (run: nota install-service)"
	}
	out, err := run("launchctl", "print", domain()+"/"+label)
	if err != nil {
		return "installed but not loaded (" + plistPath() + ")"
	}
	state := "loaded"
	for _, line := range strings.Split(out, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "state = ") {
			state = strings.TrimPrefix(l, "state = ")
			break
		}
	}
	return state + " (" + plistPath() + ")"
}
