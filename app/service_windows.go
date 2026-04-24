//go:build windows

package app

import (
	"os"
	"os/exec"

	winsvc "golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"tokentally/svc"
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
	state := "stopped"
	if status.State == winsvc.Running {
		state = "running"
	}
	return map[string]any{"installed": true, "state": state}
}

// InstallService re-launches tokentally.exe --install elevated via UAC.
func (a *App) InstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return runElevated(exe, "--install")
}

// UninstallService re-launches tokentally.exe --uninstall elevated via UAC.
func (a *App) UninstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return runElevated(exe, "--uninstall")
}

func runElevated(exe, arg string) error {
	return exec.Command("powershell", "-Command",
		"Start-Process", `"`+exe+`"`, "-ArgumentList", `"`+arg+`"`,
		"-Verb", "RunAs", "-Wait",
	).Run()
}

