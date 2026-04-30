//go:build linux

package svc

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tokentally/internal/db"
	"tokentally/internal/scanner"

	"github.com/godbus/dbus/v5"
)

const ServiceName = "tokentally"

func GetServiceStatus() map[string]any {
	conn, err := dbus.SessionBus()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	obj := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")

	var unitPath dbus.ObjectPath
	err = obj.Call("org.freedesktop.systemd1.Manager.GetUnit", 0, ServiceName+".service").Store(&unitPath)
	if err != nil {
		return map[string]any{"installed": false}
	}

	unitObj := conn.Object("org.freedesktop.systemd1", unitPath)
	prop, err := unitObj.GetProperty("org.freedesktop.systemd1.Unit.ActiveState")
	if err != nil {
		return map[string]any{"installed": true, "state": "unknown"}
	}
	state, ok := prop.Value().(string)
	if !ok {
		return map[string]any{"installed": true, "state": "unknown"}
	}

	return map[string]any{"installed": true, "state": strings.ToLower(state)}
}

func Install(exePath string) error {
	unitContent := fmt.Sprintf(`[Unit]
Description=TokenTally Service
After=graphical-session.target

[Service]
Type=simple
ExecStart=%s --service
Restart=on-failure
Environment=TOKENTALLY_DB=%s
Environment=TOKENTALLY_PROJECTS_DIR=%s

[Install]
WantedBy=graphical-session.target
`, exePath, os.Getenv("TOKENTALLY_DB"), os.Getenv("TOKENTALLY_PROJECTS_DIR"))

	userDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	destDir := filepath.Join(userDir, ".config", "systemd", "user")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating systemd user dir: %w", err)
	}

	destPath := filepath.Join(destDir, ServiceName+".service")
	if err := os.WriteFile(destPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	// Reload systemd user daemon
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	// Enable the service
	if err := exec.Command("systemctl", "--user", "enable", ServiceName+".service").Run(); err != nil {
		return fmt.Errorf("enable: %w", err)
	}

	return nil
}

func Uninstall() error {
	userDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	unitPath := filepath.Join(userDir, ".config", "systemd", "user", ServiceName+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	// Disable the service
	if err := exec.Command("systemctl", "--user", "disable", ServiceName+".service").Run(); err != nil {
		return fmt.Errorf("disable: %w", err)
	}

	// Reload systemd user daemon
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	return nil
}

func Run(pool *db.Pool, projectsDir string, interval time.Duration) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := scanner.ScanDir(pool, projectsDir); err != nil {
				log.Printf("scan error: %v", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}
