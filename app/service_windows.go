//go:build windows

package app

import (
	"os"
	"os/exec"

	"tokentally/svc"

	"golang.org/x/sys/windows/svc/mgr"
)

func (a *App) GetServiceStatus() map[string]any {
	m, err := mgr.Connect()
	if err != nil {
		return map[string]any{"installed": false, "error": err.Error()}
	}
	defer m.Disconnect()
	s, err := m.OpenService(svc.ServiceName)
	if err != nil {
		return map[string]any{"installed": false}
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return map[string]any{"installed": true, "state": "unknown"}
	}
	stateStr := "stopped"
	if status.State == 4 { // SERVICE_RUNNING
		stateStr = "running"
	}
	return map[string]any{"installed": true, "state": stateStr}
}

// InstallService re-launches tokentally.exe --install elevated via UAC.
func (a *App) InstallService() error {
	exe, _ := os.Executable()
	return runElevated(exe, "--install")
}

// UninstallService re-launches tokentally.exe --uninstall elevated via UAC.
func (a *App) UninstallService() error {
	exe, _ := os.Executable()
	return runElevated(exe, "--uninstall")
}

func runElevated(exe, arg string) error {
	return exec.Command("powershell", "-Command",
		"Start-Process", `"`+exe+`"`, "-ArgumentList", `"`+arg+`"`,
		"-Verb", "RunAs", "-Wait",
	).Run()
}
