//go:build windows

package svc

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

const ServiceName = "tokentally"

func installSCM(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %q already exists", ServiceName)
	}

	s, err = m.CreateService(ServiceName, exePath,
		mgr.Config{
			DisplayName: "TokenTally Scanner",
			Description: "Scans Claude Code JSONL transcripts and stores token usage data.",
			StartType:   mgr.StartAutomatic,
		},
		"--service",
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	return nil
}

func uninstallSCM() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %q not found: %w", ServiceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	const serviceStopDelay = 500 * time.Millisecond
	time.Sleep(serviceStopDelay)
	return nil
}
