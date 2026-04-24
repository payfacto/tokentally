//go:build windows

package svc

import (
	"database/sql"
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"tokentally/internal/scanner"
)

const ServiceName = "TokenTally"

type handler struct {
	db          *sql.DB
	projectsDir string
	interval    time.Duration
}

func newHandler(db *sql.DB, projectsDir string, interval time.Duration) *handler {
	return &handler{db: db, projectsDir: projectsDir, interval: interval}
}

func (h *handler) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	status <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue,
	}

	paused := false
	for {
		select {
		case <-ticker.C:
			if paused {
				continue
			}
			if _, err := scanner.ScanDir(h.db, h.projectsDir); err != nil {
				log.Printf("scan error: %v", err)
			}
		case c := <-req:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			case svc.Pause:
				paused = true
				status <- svc.Status{State: svc.Paused, Accepts: svc.AcceptStop | svc.AcceptPauseAndContinue}
			case svc.Continue:
				paused = false
				status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue}
			}
		}
	}
}

// Run starts the SCM service loop.
func Run(db *sql.DB, projectsDir string, interval time.Duration) error {
	h := newHandler(db, projectsDir, interval)
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return debug.Run(ServiceName, h)
	}
	return svc.Run(ServiceName, h)
}

// Install registers the service with SCM. Requires admin rights.
func Install(exePath string) error {
	elog, err := eventlog.Open(ServiceName)
	if err != nil {
		eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		elog, _ = eventlog.Open(ServiceName)
	}
	if elog != nil {
		elog.Close()
	}
	return installSCM(exePath)
}

// Uninstall removes the service from SCM. Requires admin rights.
func Uninstall() error {
	return uninstallSCM()
}
