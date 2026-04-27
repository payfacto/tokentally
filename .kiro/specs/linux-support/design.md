# Design Document: Linux Support for TokenTally

## Overview

This design document outlines the implementation of Linux support for TokenTally, a cross-platform GUI application built with Wails. The application currently supports Windows and macOS (darwin) and needs to extend support to Linux desktop environments.

The implementation will follow the existing platform-specific code patterns in the codebase, using Go build tags to separate platform-specific implementations. Key areas of focus include:

- **Tray icon support** using `systray` library (same as Windows)
- **Service management** using systemd
- **Window icon support** using X11/Wayland protocols
- **Startup registration** using systemd user services and desktop autostart entries
- **Platform detection** using Go's runtime package

## Architecture

### Current Architecture

The existing architecture uses platform-specific files with Go build tags:

```
app/
├── app.go              # Shared application logic
├── service_darwin.go   # macOS service (no-op)
├── service_windows.go  # Windows service (SCM)
├── tray_darwin.go      # macOS tray (no-op)
├── tray_windows.go     # Windows tray (systray)
└── wndicon_windows.go  # Windows window icon

main_darwin.go          # macOS entry point
main_windows.go         # Windows entry point
main_shared.go          # Shared entry point logic
```

### Linux Architecture

The Linux implementation will follow the same pattern:

```
app/
├── app.go              # Shared application logic (no changes)
├── service_darwin.go   # macOS service (no changes)
├── service_linux.go    # NEW: Linux service (systemd)
├── service_windows.go  # Windows service (no changes)
├── tray_darwin.go      # macOS tray (no changes)
├── tray_linux.go       # NEW: Linux tray (systray)
├── tray_windows.go     # Windows tray (no changes)
├── wndicon_darwin.go   # NEW: macOS window icon
├── wndicon_linux.go    # NEW: Linux window icon (X11/Wayland)
└── wndicon_windows.go  # Windows window icon (no changes)

main_darwin.go          # macOS entry point (no changes)
main_linux.go           # NEW: Linux entry point
main_shared.go          # Shared entry point logic (no changes)
```

### Platform Detection

Platform detection will use Go's standard `runtime` package:

```go
import "runtime"

func IsLinux() bool {
    return runtime.GOOS == "linux"
}
```

This will be used throughout the application to conditionally execute Linux-specific code.

## Components and Interfaces

### Platform Interface

A new platform interface will be created to abstract platform-specific functionality:

```go
// platform.go
type Platform interface {
    IsLinux() bool
    IsWindows() bool
    IsDarwin() bool
    
    GetPlatformName() string
    GetArchitecture() string
}

type platformImpl struct{}

func (p *platformImpl) IsLinux() bool     { return runtime.GOOS == "linux" }
func (p *platformImpl) IsWindows() bool   { return runtime.GOOS == "windows" }
func (p *platformImpl) IsDarwin() bool    { return runtime.GOOS == "darwin" }
func (p *platformImpl) GetPlatformName() string { return runtime.GOOS }
func (p *platformImpl) GetArchitecture() string { return runtime.GOARCH }
```

### Tray Manager Interface

The tray manager will be extended to support Linux:

```go
// tray.go
type TrayManager interface {
    StartTray()
    StopTray()
    SetTooltip(text string)
    AddMenuItem(label, tooltip string) MenuItem
    AddSeparator()
}

type MenuItem interface {
    Clicked() <-chan struct{}
    Disable()
    Enable()
    SetTitle(title string)
}
```

### Service Manager Interface

The service manager will be extended to support Linux:

```go
// service.go
type ServiceManager interface {
    GetServiceStatus() map[string]any
    InstallService() error
    UninstallService() error
    RunService(db *sql.DB, projectsDir string, interval time.Duration) error
}
```

### Window Icon Manager Interface

The window icon manager will be extended to support Linux:

```go
// window_icon.go
type WindowIconManager interface {
    SetWindowIcon(ctx context.Context)
}
```

## Data Models

No new data models are required for Linux support. The existing data models in `internal/db/` and `internal/pricing/` are platform-agnostic.

## Correctness Properties

### Property 1: Platform-specific build tags

*For any* valid Go build configuration, files with `//go:build linux` tag SHALL be compiled only when building for Linux, and SHALL NOT be compiled for Windows or macOS builds.

**Validates: Requirements 1.1, 1.2, 1.3**

### Property 2: Tray icon creation

*For any* Linux system with tray support, when TokenTally starts, the tray manager SHALL create a system tray icon with the tooltip "TokenTally".

**Validates: Requirements 2.1, 2.2**

### Property 3: Tray menu functionality

*For any* tray menu item, when the user clicks the menu item, the corresponding action (open dashboard, scan now, quit) SHALL be executed.

**Validates: Requirements 2.3, 2.4, 2.5**

### Property 4: Graceful tray degradation

*For any* Linux system without tray support, when the tray manager fails to create a tray icon, the application SHALL log a warning and continue operation without tray.

**Validates: Requirement 2.6**

### Property 5: Service installation

*For any* Linux system with systemd, when the `--install` flag is provided, the service manager SHALL register TokenTally as a systemd service named "tokentally".

**Validates: Requirement 3.1**

### Property 6: Service uninstallation

*For any* Linux system with systemd, when the `--uninstall` flag is provided, the service manager SHALL remove the TokenTally systemd service.

**Validates: Requirement 3.2**

### Property 7: Service execution

*For any* Linux system with systemd, when the `--service` flag is provided, the service manager SHALL run TokenTally as a systemd-managed service that scans every 30 seconds.

**Validates: Requirements 3.3, 3.4**

### Property 8: Service error handling

*For any* service startup failure, when the service fails to start, the service manager SHALL log the error and exit with a non-zero status code.

**Validates: Requirement 3.5**

### Property 9: Service name consistency

*For any* platform, the service name SHALL be "tokentally".

**Validates: Requirement 3.6**

### Property 10: Window icon setting

*For any* Linux system with X11 or Wayland, when the main window is created, the window manager SHALL set the window icon.

**Validates: Requirement 4.1**

### Property 11: Window icon fallback

*For any* missing icon file, when the icon file is not found, the window manager SHALL use the default system icon.

**Validates: Requirement 4.2**

### Property 12: Display server compatibility

*For any* Linux display server (X11 or Wayland), the window manager SHALL support setting the window icon.

**Validates: Requirement 4.3**

### Property 13: Startup registration

*For any* Linux system with systemd user services or desktop autostart, when the `--install` flag is provided, the startup manager SHALL register TokenTally for auto-start.

**Validates: Requirement 5.1**

### Property 14: Startup removal

*For any* Linux system with systemd user services or desktop autostart, when the `--uninstall` flag is provided, the startup manager SHALL remove the auto-start registration.

**Validates: Requirement 5.2**

### Property 15: Startup mechanism support

*For any* Linux system, the startup manager SHALL support both systemd user services and desktop autostart entries.

**Validates: Requirement 5.3**

### Property 16: Startup fallback

*For any* Linux system without systemd user services or desktop autostart, when neither mechanism is available, the startup manager SHALL log a warning and continue.

**Validates: Requirement 5.4**

### Property 17: Platform detection

*For any* Linux system, when the application starts, the platform detector SHALL identify the operating system as Linux.

**Validates: Requirements 6.1, 6.2**

### Property 18: Platform detection method

*For any* platform detection call, the platform detector SHALL use standard Go runtime detection methods.

**Validates: Requirement 6.3**

### Property 19: Build system file inclusion

*For any* Linux build, the build system SHALL include Linux-specific source files and exclude Windows and macOS-specific source files.

**Validates: Requirements 7.1, 7.2**

### Property 20: Build system architecture support

*For any* valid Go build configuration, the build system SHALL support building for Linux AMD64, ARM64, and ARMv7 architectures.

**Validates: Requirement 7.3**

### Property 21: Test framework execution

*For any* Linux test run, the test framework SHALL execute Linux-specific unit tests and skip non-Linux tests.

**Validates: Requirements 8.1, 8.4**

### Property 22: Integration test execution

*For any* Linux test run, the test framework SHALL execute integration tests for tray icon and service management functionality.

**Validates: Requirements 8.2, 8.3**

## Error Handling

### Tray Icon Errors

- **Tray initialization failure**: Log warning and continue without tray
- **Menu item creation failure**: Log error and skip menu item
- **Icon loading failure**: Log error and use default system icon

### Service Manager Errors

- **Service installation failure**: Log error and exit with non-zero status
- **Service uninstallation failure**: Log error and exit with non-zero status
- **Service execution failure**: Log error and exit with non-zero status
- **Systemd not available**: Log warning and continue without service

### Window Icon Errors

- **Icon file not found**: Log warning and use default system icon
- **X11/Wayland connection failure**: Log warning and continue without window icon

### Startup Manager Errors

- **Systemd user service registration failure**: Log warning and try desktop autostart
- **Desktop autostart registration failure**: Log warning and continue
- **Neither mechanism available**: Log warning and continue

## Testing Strategy

### Unit Tests

Platform-specific unit tests will be created for each platform-specific component:

```go
// app/tray_linux_test.go
func TestTrayLinux_CreateTrayIcon(t *testing.T) {
    // Test tray icon creation on Linux
}

func TestTrayLinux_MenuItemClick(t *testing.T) {
    // Test menu item click handling
}

// app/service_linux_test.go
func TestServiceLinux_InstallService(t *testing.T) {
    // Test service installation
}

func TestServiceLinux_UninstallService(t *testing.T) {
    // Test service uninstallation
}

// app/wndicon_linux_test.go
func TestWindowIconLinux_SetWindowIcon(t *testing.T) {
    // Test window icon setting
}
```

### Integration Tests

Integration tests will be created for Linux-specific functionality:

```go
// integration/linux_test.go
func TestLinux_AppStartup(t *testing.T) {
    // Test full application startup on Linux
}

func TestLinux_TrayIcon(t *testing.T) {
    // Test tray icon visibility and functionality
}

func TestLinux_ServiceManagement(t *testing.T) {
    // Test service installation and management
}
```

### Test Configuration

Tests will be tagged to enable platform-specific test execution:

```bash
# Run all tests
go test ./...

# Run Linux-specific tests only
go test -tags linux ./...

# Run tests on Linux with verbose output
go test -v -tags linux ./...
```

### Manual Testing Checklist

- [ ] Application builds successfully for Linux AMD64, ARM64, and ARMv7
- [ ] Application starts and displays the main window
- [ ] Window icon is displayed correctly
- [ ] System tray icon appears and is functional
- [ ] Service can be installed and uninstalled
- [ ] Service starts automatically at boot
- [ ] Auto-start registration works for user sessions
- [ ] Scanning functionality works correctly
- [ ] Application exits cleanly

## Implementation Notes

### Build Tags

All Linux-specific files will use the `//go:build linux` build tag:

```go
//go:build linux

package app
```

### File Organization

Platform-specific files will be organized as follows:

```
app/
├── app.go              # Shared application logic
├── service_darwin.go   # macOS service (no-op)
├── service_linux.go    # Linux service (systemd)
├── service_windows.go  # Windows service (SCM)
├── tray_darwin.go      # macOS tray (no-op)
├── tray_linux.go       # Linux tray (systray)
├── tray_windows.go     # Windows tray (systray)
├── wndicon_darwin.go   # macOS window icon
├── wndicon_linux.go    # Linux window icon (X11/Wayland)
└── wndicon_windows.go  # Windows window icon (WM_SETICON)

main_darwin.go          # macOS entry point
main_linux.go           # Linux entry point
main_shared.go          # Shared entry point logic
```

### Dependencies

The following dependencies will be used:

- `github.com/getlantern/systray` - Tray icon support (same as Windows)
- `github.com/godbus/dbus/v5` - D-Bus communication for systemd
- `github.com/godbus/dbus/v5/introspect` - D-Bus introspection

### Cross-Compilation

The application will support cross-compilation for Linux from Windows and macOS hosts:

```bash
# Build for Linux AMD64 from macOS
GOOS=linux GOARCH=amd64 wails build

# Build for Linux ARM64 from Windows
GOOS=linux GOARCH=arm64 wails build

# Build for Linux ARMv7 from Linux
GOOS=linux GOARCH=arm GOARM=7 wails build
```

### Platform-Specific Considerations

#### Desktop Environments

Linux has many desktop environments (GNOME, KDE, XFCE, etc.) with varying levels of tray icon support. The implementation will gracefully handle environments without tray support.

#### Display Servers

Both X11 and Wayland are in use. The implementation will work with both, with X11 being the primary target due to broader systray support.

#### Service Managers

While systemd is dominant, some distributions use alternative init systems. The service installation will detect systemd availability and provide appropriate error messages.

### Security Considerations

- **Service Installation**: Installing system services requires root/administrative privileges. The application will clearly indicate when elevated privileges are needed.
- **User Services**: For non-root installation, systemd user services will be supported as an alternative.

### User Experience

- **Tray Icon Visibility**: Some Linux distributions hide tray icons by default or require configuration. The application will provide clear instructions if tray icons are not visible.
- **Service Management**: Users will be able to manage the service using standard tools like `systemctl`.
