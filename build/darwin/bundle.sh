#!/bin/sh
# Assembles build/bin/TokenTally.app from a fresh `go build`.
#
# Replaces what Wails v2's `wails build` CLI did silently; see
# .context/specs/2026-08-09-wails-v3-build-packaging-design.md.
#
# Usage: build/darwin/bundle.sh <version> [numeric-version] [ldflags-version-var]
#   <version>         raw display string, e.g. "v1.2.3-4-gabcdef-dirty"
#   [numeric-version]  Apple-conformant X.Y.Z for CFBundleVersion/
#                       CFBundleShortVersionString; derived from <version> if omitted
set -eu

VERSION="${1:?usage: bundle.sh <version> [numeric-version] [ldflags-version-var]}"
NUMERIC_VERSION="${2:-}"
VERSION_VAR="${3:-tokentally/internal/version.Version}"

if [ -z "$NUMERIC_VERSION" ]; then
	NUMERIC_VERSION="$(printf '%s' "$VERSION" | sed -E 's/^v//; s/[-+].*//')"
	case "$NUMERIC_VERSION" in
		[0-9]*.[0-9]*.[0-9]*) ;;
		*) NUMERIC_VERSION="0.0.0" ;;
	esac
fi

if [ "$(go env GOOS)" != "darwin" ]; then
	echo "bundle.sh: must run on macOS (host GOOS=$(go env GOOS))" >&2
	exit 1
fi
GOARCH="${GOARCH:-$(go env GOARCH)}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
APP_DIR="$ROOT/build/bin/TokenTally.app"
CONTENTS="$APP_DIR/Contents"

rm -rf "$APP_DIR"
mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources"

(cd "$ROOT" && GOARCH="$GOARCH" go build -ldflags "-X '$VERSION_VAR=$VERSION'" -o "$CONTENTS/MacOS/tokentally" .)

sed -e "s|__VERSION__|$VERSION|g" -e "s|__NUMERIC_VERSION__|$NUMERIC_VERSION|g" "$ROOT/build/darwin/Info.plist" > "$CONTENTS/Info.plist"
cp "$ROOT/build/darwin/icon.icns" "$CONTENTS/Resources/icon.icns"

echo "Bundled $APP_DIR (version $VERSION, GOARCH=$GOARCH)"
