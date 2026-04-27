//go:build linux

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"tokentally/app"
	"tokentally/internal/db"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var rawAssets embed.FS

func main() {
	installFlag := flag.Bool("install", false, "Install Linux systemd user service")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Linux systemd user service")
	serviceFlag := flag.Bool("service", false, "Run as Linux systemd service (internal use)")
	flag.Parse()

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

	// Start tray in a goroutine
	go a.StartTray()

	if err := wails.Run(&options.App{
		Title:             "TokenTally",
		Width:             1100,
		Height:            700,
		MinWidth:          800,
		MinHeight:         600,
		BackgroundColour:  &options.RGBA{R: 13, G: 13, B: 26, A: 255},
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  a.Startup,
		OnDomReady: a.SetWindowIcon,
		Bind:       []any{a},
	}); err != nil {
		log.Printf("wails: %v", err)
	}
	os.Exit(0)
}
