//go:build windows

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"tokentally/app"
	"tokentally/internal/db"
	"tokentally/internal/pricing"
	"tokentally/svc"
)

//go:embed all:frontend
var rawAssets embed.FS

//go:embed pricing.json
var rawPricing embed.FS

//go:embed build/windows/icon.ico
var iconICO []byte

func main() {
	installFlag := flag.Bool("install", false, "Install Windows service (requires admin)")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Windows service (requires admin)")
	serviceFlag := flag.Bool("service", false, "Run as Windows SCM service (internal use)")
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
	exe, _ := os.Executable()
	if err := svc.Install(exe); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		os.Exit(1)
	}
	addToStartup()
	fmt.Println("TokenTally service installed.")
}

func runUninstall() {
	if err := svc.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall: %v\n", err)
		os.Exit(1)
	}
	removeFromStartup()
	fmt.Println("TokenTally service uninstalled.")
}

func runService(dbPath, projectsDir string, interval time.Duration) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()
	if err := svc.Run(conn, projectsDir, interval); err != nil {
		log.Fatalf("svc.Run: %v", err)
	}
}

func runUI(dbPath, projectsDir string) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	p := loadPricing()
	a := app.New(conn, projectsDir, p)
	app.IconBytes = iconICO

	assets, _ := fs.Sub(rawAssets, "frontend")

	// systray locks its own OS thread internally, so it can run in a goroutine.
	// Wails stays on the main goroutine for the most stable WebView2 message loop.
	go a.StartTray()

	err = wails.Run(&options.App{
		Title:             "TokenTally",
		Width:             1100,
		Height:            700,
		MinWidth:          800,
		MinHeight:         600,
		BackgroundColour:  &options.RGBA{R: 13, G: 13, B: 26, A: 255},
		HideWindowOnClose: true, // keep runtime alive so tray can re-show the window
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.Startup,
		Bind:      []any{a},
	})
	if err != nil {
		log.Printf("wails: %v", err)
	}
	// Wails exited (Ctrl+C, runtime.Quit, or error); kill the systray goroutine too.
	os.Exit(0)
}

func loadPricing() *pricing.Pricing {
	if override := os.Getenv("TOKENTALLY_PRICING_JSON"); override != "" {
		f, err := os.Open(override)
		if err == nil {
			p, _ := pricing.Load(f)
			f.Close()
			return p
		}
	}
	f, err := rawPricing.Open("pricing.json")
	if err != nil {
		return nil
	}
	defer f.Close()
	p, _ := pricing.Load(f)
	return p
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func addToStartup() {
	exe, _ := os.Executable()
	key := `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runCmd("reg", "add", key, "/v", "TokenTally", "/t", "REG_SZ", "/d", exe, "/f")
}

func removeFromStartup() {
	key := `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runCmd("reg", "delete", key, "/v", "TokenTally", "/f")
}

func runCmd(name string, args ...string) {
	exec.Command(name, args...).Run()
}
