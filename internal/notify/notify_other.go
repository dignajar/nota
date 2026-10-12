//go:build !darwin && !linux

package notify

import (
	"fmt"
	"runtime"
)

type unsupported struct{}

func (unsupported) Send(string, string) error {
	return fmt.Errorf("notifications are not supported on %s; set NOTA_NOTIFY_CMD", runtime.GOOS)
}
func (unsupported) Describe() string { return "none (unsupported OS)" }

func platform() Notifier { return unsupported{} }
