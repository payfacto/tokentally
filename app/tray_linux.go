//go:build linux

package app

import (
	"os"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartTray initializes the system tray on Linux.
func (a *App) StartTray() {
	systray.Run(a.onTrayReady, func() {})
}

func (a *App) onTrayReady() {
	systray.SetTooltip("TokenTally")

	mOpen := systray.AddMenuItem("Open Dashboard", "Open the TokenTally window")
	mScan := systray.AddMenuItem("Scan Now", "Trigger an immediate scan")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit TokenTally", "Exit TokenTally")

	for {
		select {
		case <-mOpen.ClickedCh:
			if a.ctx != nil {
				runtime.WindowShow(a.ctx)
				runtime.WindowUnminimise(a.ctx)
			}
		case <-mScan.ClickedCh:
			go a.ScanNow() //nolint:errcheck
		case <-mQuit.ClickedCh:
			os.Exit(0)
		}
	}
}
