//go:build linux

package app

// SetWindowIcon is a no-op on Linux. Wails has no runtime API for setting
// the window icon after the window is shown — it must be passed at startup
// via WebviewWindowOptions. Kept here for cross-platform symmetry with the
// Windows WindowRuntimeReady hook.
func (a *App) SetWindowIcon() {}
