//go:build !windows

package app

// No taskbar to adjust outside Windows.
func startTaskbarIdentity() {}
