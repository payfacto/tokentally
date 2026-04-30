//go:build linux

package app

import (
	"fmt"
	"os"
	"time"

	"tokentally/internal/db"
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
func (a *App) RunService(pool *db.Pool, projectsDir string, interval time.Duration) error {
	return svc.Run(pool, projectsDir, interval)
}
