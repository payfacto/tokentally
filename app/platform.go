package app

import "runtime"

// Platform provides platform detection functions.
type Platform struct{}

// IsLinux returns true if the current platform is Linux.
func (p *Platform) IsLinux() bool {
	return runtime.GOOS == "linux"
}

// IsWindows returns true if the current platform is Windows.
func (p *Platform) IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsDarwin returns true if the current platform is macOS (darwin).
func (p *Platform) IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

// GetPlatformName returns the current OS name.
func (p *Platform) GetPlatformName() string {
	return runtime.GOOS
}

// GetArchitecture returns the current architecture.
func (p *Platform) GetArchitecture() string {
	return runtime.GOARCH
}
