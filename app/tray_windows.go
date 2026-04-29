//go:build windows

package app

import (
	"os"
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// IconBytes is set by main_windows.go from the embedded icon.ico.
var IconBytes []byte

// StartTray initialises the system tray. Must be called from the main goroutine.
func (a *App) StartTray() {
	systray.Run(a.onTrayReady, func() {})
}

const focusPulseDelay = 150 * time.Millisecond

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
				// Run in a goroutine — Wails window calls can block via cross-thread
				// Win32 SendMessage if WebView2 is busy, which would freeze the tray loop.
				go func() {
					runtime.WindowShow(a.ctx)
					runtime.WindowUnminimise(a.ctx)
					runtime.WindowSetAlwaysOnTop(a.ctx, true)
					time.Sleep(focusPulseDelay)
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
