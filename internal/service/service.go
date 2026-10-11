// Package service installs the nota daemon as a per user background service:
// a systemd user unit on Linux, a launchd agent on macOS.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager installs and inspects the background service.
type Manager interface {
	// Install writes the service definition and starts it. It returns the
	// path of the file it wrote.
	Install(binary, dataDir string) (string, error)
	Uninstall() error
	// Status is a one line human readable state.
	Status() string
}

// New returns the manager for the current OS.
func New() Manager { return platform() }

// Binary returns the absolute, symlink resolved path of the running
// executable, refusing temporary `go run` builds.
func Binary() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	if strings.Contains(p, string(filepath.Separator)+"go-build") {
		return "", fmt.Errorf("running from a temporary `go run` build (%s); install the binary first (make install)", p)
	}
	return p, nil
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, s)
		}
		return s, fmt.Errorf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return s, nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

type unsupported struct{}

func (unsupported) Install(string, string) (string, error) {
	return "", fmt.Errorf("service install is only supported on Linux (systemd) and macOS (launchd); run `nota daemon` yourself")
}
func (unsupported) Uninstall() error { return unsupported{}.err() }
func (unsupported) Status() string   { return "unsupported OS" }
func (unsupported) err() error       { return fmt.Errorf("unsupported OS") }
