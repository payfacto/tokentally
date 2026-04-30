# TokenTally — version-stamped build helpers.
#
# VERSION is derived from the nearest git tag. Override with:
#   make build VERSION=v1.2.3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X 'tokentally/internal/version.Version=$(VERSION)'

.PHONY: build build-windows build-darwin build-linux test clean version

build:
	wails build -ldflags "$(LDFLAGS)"

build-windows:
	wails build -platform windows/amd64 -ldflags "$(LDFLAGS)"

build-darwin:
	wails build -platform darwin/arm64 -ldflags "$(LDFLAGS)"

build-linux:
	wails build -platform linux/amd64 -ldflags "$(LDFLAGS)"

test:
	go test ./...

clean:
	rm -rf build/bin

version:
	@echo $(VERSION)
