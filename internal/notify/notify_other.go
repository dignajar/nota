//go:build !darwin && !linux

package notify

func platform() Notifier { return unsupported{} }
