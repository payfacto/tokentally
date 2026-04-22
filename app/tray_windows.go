//go:build windows

package app

import (
	"os"
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// IconBytes is set by main.go from the embedded icon.ico.
var IconBytes []byte

// StartTray initialises the system tray. Must be called from the main goroutine.
func (a *App) StartTray() {
	systray.Run(a.onTrayReady, func() {})
}

func (a *App) onTrayReady() {
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
			if a.ctx != nil {
				runtime.WindowShow(a.ctx)
				runtime.WindowUnminimise(a.ctx)
				// Brief always-on-top pulse forces the window to the foreground on Windows.
				runtime.WindowSetAlwaysOnTop(a.ctx, true)
				go func() {
					time.Sleep(150 * time.Millisecond)
					runtime.WindowSetAlwaysOnTop(a.ctx, false)
				}()
			}
		case <-mScan.ClickedCh:
			go a.ScanNow() //nolint:errcheck
		case <-mQuit.ClickedCh:
			// os.Exit is the only reliable way to quit when systray owns the main
			// thread; runtime.Quit + systray.Quit can deadlock on Windows.
			os.Exit(0)
		}
	}
}
