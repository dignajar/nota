//go:build !darwin && !linux

package service

func platform() Manager { return unsupported{} }
