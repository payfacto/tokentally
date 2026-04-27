//go:build windows

package svc

import (
	"database/sql"
	"log"
	"time"

	"tokentally/internal/scanner"
)

// Install installs the Windows service.
func Install(exePath string) error {
	return installSCM(exePath)
}

// Uninstall uninstalls the Windows service.
func Uninstall() error {
	return uninstallSCM()
}

// Run runs the application as a Windows service.
func Run(db *sql.DB, projectsDir string, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := scanner.ScanDir(db, projectsDir); err != nil {
				log.Printf("scan error: %v", err)
			}
		}
	}
}
