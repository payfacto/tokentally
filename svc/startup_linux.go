//go:build linux

package svc

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	autostartFile = "tokentally.desktop"
)

func InstallStartup(exePath string) error {
	// Try systemd user service first
	if err := Install(exePath); err != nil {
		// Log warning but continue to desktop autostart
		fmt.Printf("systemd user service install failed: %v; trying desktop autostart\n", err)
	}

	// Fallback to desktop autostart entry
	return installAutostart(exePath)
}

func UninstallStartup() error {
	// Try systemd user service first
	if err := Uninstall(); err != nil {
		fmt.Printf("systemd user service uninstall failed: %v; trying desktop autostart\n", err)
	}

	// Fallback to desktop autostart entry
	return uninstallAutostart()
}

func installAutostart(exePath string) error {
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=TokenTally
Comment=Token usage tracker for Claude Code
Exec=%s --service
StartupNotify=false
Terminal=false
Icon=tokentally
`, exePath)

	userDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	destDir := filepath.Join(userDir, ".config", "autostart")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating autostart dir: %w", err)
	}

	destPath := filepath.Join(destDir, autostartFile)
	if err := os.WriteFile(destPath, []byte(desktopContent), 0644); err != nil {
		return fmt.Errorf("writing autostart file: %w", err)
	}

	return nil
}

func uninstallAutostart() error {
	userDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	autostartPath := filepath.Join(userDir, ".config", "autostart", autostartFile)
	if err := os.Remove(autostartPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing autostart file: %w", err)
	}

	return nil
}
