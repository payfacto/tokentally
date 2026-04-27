//go:build linux

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetWindowIcon sets the window icon for Linux (X11/Wayland).
// On Linux with Wails, we load the icon and set it via the window options.
func (a *App) SetWindowIcon(ctx context.Context) {
	iconPath := filepath.Join("assets", "icon.png")

	// Try to load the icon file
	data, err := os.ReadFile(iconPath)
	if err != nil {
		// Fallback: use default system icon
		fmt.Printf("window icon: icon file not found, using default: %v\n", err)
		return
	}

	// Set the window icon via Wails runtime
	if err := runtime.SetWindowIconFromBytes(ctx, data); err != nil {
		fmt.Printf("window icon: failed to set icon: %v\n", err)
	}
}
