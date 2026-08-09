//go:build linux

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"tokentally/app"
	"tokentally/internal/db"
	"tokentally/internal/version"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	windowWidth     = 1100
	windowHeight    = 700
	windowMinWidth  = 800
	windowMinHeight = 600

	bgR = 13
	bgG = 13
	bgB = 26
)

//go:embed all:frontend
var rawAssets embed.FS

func main() {
	installFlag := flag.Bool("install", false, "Install Linux systemd user service")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Linux systemd user service")
	serviceFlag := flag.Bool("service", false, "Run as Linux systemd service (internal use)")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("TokenTally version %s\n", version.Version)
		return
	}

	dbPath := envOrDefault("TOKENTALLY_DB", filepath.Join(homeDir(), ".claude", "tokentally.db"))
	projectsDir := envOrDefault("TOKENTALLY_PROJECTS_DIR", filepath.Join(homeDir(), ".claude", "projects"))
	scanInterval := 30 * time.Second

	switch {
	case *installFlag:
		runInstall()
	case *uninstallFlag:
		runUninstall()
	case *serviceFlag:
		runService(dbPath, projectsDir, scanInterval)
	default:
		runUI(dbPath, projectsDir)
	}
}

func runInstall() {
	a := &app.App{}
	if err := a.InstallStartup(); err != nil {
		log.Printf("install startup: %v", err)
	}
	if err := a.InstallService(); err != nil {
		log.Printf("install service: %v", err)
	}
	log.Println("TokenTally installed.")
}

func runUninstall() {
	a := &app.App{}
	if err := a.UninstallStartup(); err != nil {
		log.Printf("uninstall startup: %v", err)
	}
	if err := a.UninstallService(); err != nil {
		log.Printf("uninstall service: %v", err)
	}
	log.Println("TokenTally uninstalled.")
}

func runService(dbPath, projectsDir string, interval time.Duration) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	p := loadPricing()
	a := app.New(conn, projectsDir, p)

	// Run as service
	if err := a.RunService(conn, projectsDir, interval); err != nil {
		log.Fatalf("service: %v", err)
	}
}

func runUI(dbPath, projectsDir string) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	p := loadPricing()
	a := app.New(conn, projectsDir, p)

	assets, err := fs.Sub(rawAssets, "frontend")
	if err != nil {
		log.Fatalf("assets: %v", err)
	}

	wailsApp := application.New(application.Options{
		Name:     "TokenTally",
		Services: []application.Service{application.NewService(a)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "TokenTally",
		Width:            windowWidth,
		Height:           windowHeight,
		MinWidth:         windowMinWidth,
		MinHeight:        windowMinHeight,
		BackgroundColour: application.NewRGBA(bgR, bgG, bgB, 255),
	})

	// Hide instead of quit on close so the tray can re-show the window.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(e *application.WindowEvent) {
		a.SetWindowIcon()
	})

	a.SetupTray(wailsApp, window, nil)

	if err := wailsApp.Run(); err != nil {
		log.Printf("wails: %v", err)
	}
}
