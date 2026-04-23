//go:build darwin

package app

// IconBytes is unused on macOS (no systray).
var IconBytes []byte

// StartTray is a no-op on macOS; systray requires the OS main thread which
// Wails already owns for WebKit.
func (a *App) StartTray() {}
