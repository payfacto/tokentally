package version

// Version is set at build time via -ldflags:
//
//	go build -ldflags "-X tokentally/internal/version.Version=v1.2.3"
//
// Defaults to "dev" for plain go/wails build invocations.
var Version = "dev"
