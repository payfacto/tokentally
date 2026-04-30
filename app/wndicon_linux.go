//go:build linux

package app

import "context"

// SetWindowIcon is a no-op on Linux. Wails v2 has no runtime API for
// setting the window icon after the window is shown — it must be passed
// at startup via options.App.Linux.Icon. Kept here for cross-platform
// symmetry with the Windows OnDomReady hook.
func (a *App) SetWindowIcon(_ context.Context) {}
