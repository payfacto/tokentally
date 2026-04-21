//go:build windows

package app

import (
	"os"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// IconBytes is set by main.go from the embedded icon.png.
var IconBytes []byte

// StartTray initialises the system tray. Must be called from the main goroutine.
// wailsShow is a func that shows/focuses the Wails window.
func (a *App) StartTray(wailsShow func()) {
	systray.Run(
		func() { a.onTrayReady(wailsShow) },
		func() { /* on exit */ },
	)
}

func (a *App) onTrayReady(wailsShow func()) {
	if len(IconBytes) > 0 {
		systray.SetIcon(IconBytes)
	}
	systray.SetTooltip("TokenTally")

	mOpen := systray.AddMenuItem("Open Dashboard", "Open the TokenTally window")
	mScan := systray.AddMenuItem("Scan Now", "Trigger an immediate scan")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit TokenTally", "Exit TokenTally")

	for {
		select {
		case <-mOpen.ClickedCh:
			wailsShow()
		case <-mScan.ClickedCh:
			go a.ScanNow() //nolint:errcheck
		case <-mQuit.ClickedCh:
			systray.Quit()
			if a.ctx != nil {
				runtime.Quit(a.ctx)
			}
			os.Exit(0)
		}
	}
}
