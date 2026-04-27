//go:build linux

package app

import (
	"fmt"
	"os"

	"tokentally/svc"
)

// InstallStartup registers the application for auto-start on Linux.
func (a *App) InstallStartup() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable: %w", err)
	}
	return svc.InstallStartup(exe)
}

// UninstallStartup removes the auto-start registration on Linux.
func (a *App) UninstallStartup() error {
	return svc.UninstallStartup()
}
