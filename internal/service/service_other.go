//go:build !darwin && !linux

package service

import "fmt"

type unsupported struct{}

func (unsupported) Install(string, string) (string, error) {
	return "", fmt.Errorf("service install is only supported on Linux (systemd) and macOS (launchd); run `nota daemon` yourself")
}
func (unsupported) Uninstall() error { return unsupported{}.err() }
func (unsupported) Status() string   { return "unsupported OS" }
func (unsupported) err() error       { return fmt.Errorf("unsupported OS") }

func platform() Manager { return unsupported{} }
