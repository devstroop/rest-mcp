//go:build windows
// +build windows

package main

// watchReload is a no-op on Windows since SIGHUP is not available.
// Windows users should restart the process to reload configuration.
func watchReload(reloadFn func()) {
	// No-op: SIGHUP not supported on Windows.
}
