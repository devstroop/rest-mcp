//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/devstroop/rest-mcp/internal/logger"
)

// watchReload listens for SIGHUP and triggers a config reload.
// On Unix systems, sending SIGHUP to the process causes it to
// reload its configuration, re-parse operations, and update tools.
func watchReload(reloadFn func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	go func() {
		for range sigCh {
			logger.Info("received SIGHUP, reloading configuration")
			reloadFn()
		}
	}()
}
