//go:build linux

package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SetupTray wires the Linux notification-area icon, tooltip, and menu.
// Must be called before wailsApp.Run() so the tray exists once the event
// loop starts; v3's system tray is native, so no manual OS-thread handling
// is needed here (unlike the old getlantern/systray + goroutine dance).
func (a *App) SetupTray(wailsApp *application.App, window *application.WebviewWindow, iconBytes []byte) {
	tray := wailsApp.SystemTray.New()
	if len(iconBytes) > 0 {
		tray.SetIcon(iconBytes)
	}
	tray.SetTooltip("TokenTally")

	menu := wailsApp.NewMenu()
	menu.Add("Open Dashboard").OnClick(func(_ *application.Context) {
		window.Show()
		window.UnMinimise()
		window.Focus()
	})
	menu.Add("Scan Now").OnClick(func(_ *application.Context) {
		go a.ScanNow() //nolint:errcheck
	})
	menu.AddSeparator()
	menu.Add("Quit TokenTally").OnClick(func(_ *application.Context) {
		wailsApp.Quit()
	})
	tray.SetMenu(menu)
}
