//go:build linux

package app

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"tokentally/svc"
)

// GetServiceStatus returns the current service status.
func (a *App) GetServiceStatus() map[string]any {
	return svc.GetServiceStatus()
}

// InstallService installs the Linux systemd user service.
func (a *App) InstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable: %w", err)
	}
	return svc.Install(exe)
}

// UninstallService uninstalls the Linux systemd user service.
func (a *App) UninstallService() error {
	return svc.Uninstall()
}

// RunService runs the application as a Linux systemd service.
func (a *App) RunService(db *sql.DB, projectsDir string, interval time.Duration) error {
	return svc.Run(db, projectsDir, interval)
}
