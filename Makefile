# TokenTally — version-stamped build helpers.
#
# VERSION is derived from the nearest git tag. Override with:
#   make build VERSION=v1.2.3
#
# Packaging no longer goes through `wails build` (Wails v3's CLI has no
# equivalent - `wails3 build`/`wails3 dev` require a Taskfile/build/config.yml
# scaffold this repo doesn't have). Instead: plain `go build` plus wails3's
# standalone `generate` subcommands for the platform-specific pieces Wails v2
# used to do silently. See
# .context/specs/2026-08-09-wails-v3-build-packaging-design.md.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X 'tokentally/internal/version.Version=$(VERSION)'
HOST_OS := $(shell go env GOOS)

# Apple/Windows version fields (CFBundleVersion, .syso fixed file/product
# version, manifest assemblyIdentity) must be numeric dotted quads/triples -
# git describe's "v1.2.3-4-gabcdef-dirty" isn't one. Derive an X.Y.Z from it,
# falling back to 0.0.0 for anything that doesn't look like a semver tag
# (e.g. the "dev" default with no tags at all).
NUMERIC_VERSION := $(shell v=$$(printf '%s' '$(VERSION)' | sed -E 's/^v//; s/[-+].*//'); echo "$$v" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' && echo "$$v" || echo '0.0.0')

.PHONY: build build-windows build-darwin build-linux frontend test doc-lint clean version

build:
ifeq ($(HOST_OS),darwin)
	$(MAKE) build-darwin
else ifeq ($(HOST_OS),windows)
	$(MAKE) build-windows
else
	$(MAKE) build-linux
endif

# frontend/web/app.bundle.js and app.css are gitignored build output (embedded
# into the Go binary via //go:embed all:frontend) - Wails v2's CLI rebuilt
# them automatically as part of `wails build`; plain `go build` does not, so
# every platform target depends on this first.
frontend:
	npm install --prefix frontend/inspector
	npm run build --prefix frontend/inspector

# macOS: assembles build/bin/TokenTally.app (binary, Info.plist, icon.icns).
build-darwin: frontend
	build/darwin/bundle.sh "$(VERSION)" "$(NUMERIC_VERSION)"

# Windows: generates a .syso resource (icon/version-info/manifest) that
# `go build` auto-links because it's named wails_windows_amd64.syso, next to
# main_windows.go. Requires the wails3 CLI (`go install
# github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.5`).
build-windows: frontend
	mkdir -p build/bin
	info=$$(mktemp) && \
	sed -e "s|__VERSION__|$(VERSION)|g" -e "s|__NUMERIC_VERSION__|$(NUMERIC_VERSION)|g" build/windows/info.json > "$$info" && \
	manifest=$$(mktemp) && \
	sed -e "s|__NUMERIC_VERSION__|$(NUMERIC_VERSION)|g" build/windows/wails.exe.manifest > "$$manifest" && \
	wails3 generate syso -icon build/windows/icon.ico -info "$$info" -manifest "$$manifest" -out wails_windows_amd64.syso -arch amd64; \
	status=$$?; rm -f "$$info" "$$manifest"; \
	if [ $$status -ne 0 ]; then rm -f wails_windows_amd64.syso; exit $$status; fi
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/tokentally.exe . || { rm -f wails_windows_amd64.syso; exit 1; }
	rm -f wails_windows_amd64.syso

# Linux: raw binary (unchanged - needed for the existing --install systemd/
# autostart flow) plus a .AppImage as an additional distribution artifact,
# canonically renamed to match what CI publishes. AppImage generation needs
# libgtk-4-dev, libwebkitgtk-6.0-dev, dpkg-dev, fuse, file, and the wails3 CLI
# on the build machine.
build-linux: frontend
	mkdir -p build/bin
	cp build/appicon.png build/linux/tokentally.png
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/tokentally .
	wails3 generate appimage -binary build/bin/tokentally -desktopfile build/linux/tokentally.desktop -icon build/linux/tokentally.png -outputdir build/bin
	mv build/bin/*.AppImage build/bin/tokentally-linux-amd64.AppImage

test:
	go test ./...

doc-lint:
	node scripts/doc-lint.mjs

clean:
	rm -rf build/bin

version:
	@echo $(VERSION)
