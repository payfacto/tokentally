//go:build darwin

package app

import "errors"

func (a *App) GetServiceStatus() map[string]any {
	return map[string]any{"installed": false, "error": "not supported on macOS"}
}

func (a *App) InstallService() error {
	return errors.New("not supported on macOS")
}

func (a *App) UninstallService() error {
	return errors.New("not supported on macOS")
}
