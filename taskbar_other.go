//go:build !windows

package main

// No taskbar to adjust outside Windows.
func startTaskbarIdentity() {}
