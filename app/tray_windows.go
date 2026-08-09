//go:build windows

package app

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const focusPulseDelay = 150 * time.Millisecond

// SetupTray wires the Windows notification-area icon, tooltip, and menu.
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
		// Run in a goroutine — window calls can block on the WebView2 message
		// loop, which would freeze tray click handling.
		go func() {
			window.Show()
			window.UnMinimise()
			window.SetAlwaysOnTop(true)
			time.Sleep(focusPulseDelay)
			window.SetAlwaysOnTop(false)
			window.Focus()
		}()
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
