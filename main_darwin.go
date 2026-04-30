//go:build darwin

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"tokentally/app"
	"tokentally/internal/db"
	"tokentally/internal/version"
)

//go:embed all:frontend
var rawAssets embed.FS

func main() {
	// Service management flags are Windows-only; accepted but ignored on macOS
	// so that cross-platform scripts don't break.
	flag.Bool("install", false, "")
	flag.Bool("uninstall", false, "")
	flag.Bool("service", false, "")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("TokenTally version %s\n", version.Version)
		return
	}

	dbPath := envOrDefault("TOKENTALLY_DB", filepath.Join(homeDir(), ".claude", "tokentally.db"))
	projectsDir := envOrDefault("TOKENTALLY_PROJECTS_DIR", filepath.Join(homeDir(), ".claude", "projects"))

	runUI(dbPath, projectsDir)
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

	if err := wails.Run(&options.App{
		Title:            "TokenTally",
		Width:            1100,
		Height:           700,
		MinWidth:         800,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 13, G: 13, B: 26, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.Startup,
		Bind:      []any{a},
	}); err != nil {
		log.Printf("wails: %v", err)
	}
}
