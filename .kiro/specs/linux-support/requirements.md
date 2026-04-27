# Requirements Document

## Introduction

This feature adds Linux support to TokenTally, extending the application beyond Windows and macOS (darwin). TokenTally is a cross-platform GUI application built with Wails that scans Claude Code JSONL transcripts and stores token usage data. Currently, the application supports Windows (with service management, tray icon, and startup registration) and macOS (with minimal platform-specific code due to Wails managing the main thread). Linux support requires implementing equivalent functionality for Linux desktop environments and service management systems.

## Glossary

- **TokenTally**: The cross-platform GUI application for scanning Claude Code JSONL transcripts and storing token usage data
- **Linux**: The target operating system platform for this feature
- **X11**: The X Window System, the standard graphical windowing system on most Linux distributions
- **Wayland**: A modern display server protocol that may replace X11
- **systemd**: The system and service manager used by most modern Linux distributions
- **D-Bus**: A message bus system that provides inter-process communication
- **systray**: A Go library for creating system tray icons (currently used on Windows)
- **Wails**: The cross-platform GUI framework used by TokenTally

## Requirements

### Requirement 1: Platform-Specific Build Support

**User Story:** As a developer, I want to add Linux-specific build tags, so that platform-specific code is compiled only on Linux.

#### Acceptance Criteria

1. WHEN a Linux build is initiated, THE Build System SHALL compile files with the `//go:build linux` build tag
2. WHEN a non-Linux build is initiated, THE Build System SHALL NOT compile files with the `//go:build linux` build tag
3. THE Build System SHALL support cross-compilation for Linux from Windows and macOS hosts

### Requirement 2: Tray Icon Support

**User Story:** As a Linux user, I want a system tray icon, so that I can access TokenTally features without keeping the main window open.

#### Acceptance Criteria

1. WHEN TokenTally starts on Linux, THE Tray Manager SHALL create a system tray icon
2. WHILE the tray icon exists, THE Tray Manager SHALL display a tooltip with the application name "TokenTally"
3. WHEN the "Open Dashboard" menu item is clicked, THE Tray Manager SHALL show and unminimize the main window
4. WHEN the "Scan Now" menu item is clicked, THE Tray Manager SHALL trigger an immediate scan
5. WHEN the "Quit TokenTally" menu item is clicked, THE Tray Manager SHALL exit the application
6. IF the system does not support system tray icons, THEN THE Tray Manager SHALL log a warning and continue operation without tray

### Requirement 3: Service Management

**User Story:** As a Linux user, I want to install TokenTally as a system service, so that it runs automatically at boot and scans periodically.

#### Acceptance Criteria

1. WHEN the `--install` flag is provided, THE Service Manager SHALL register TokenTally as a systemd service
2. WHEN the `--uninstall` flag is provided, THE Service Manager SHALL remove the TokenTally systemd service
3. WHEN the `--service` flag is provided, THE Service Manager SHALL run TokenTally as a systemd-managed service
4. WHILE running as a service, THE Scanner SHALL scan the projects directory every 30 seconds
5. IF the service fails to start, THEN THE Service Manager SHALL log the error and exit with a non-zero status code
6. THE Service Name SHALL be "tokentally" for consistency across platforms

### Requirement 4: Window Icon Support

**User Story:** As a Linux user, I want the application window to display the TokenTally icon, so that the application is visually consistent with the platform.

#### Acceptance Criteria

1. WHEN the main window is created, THE Window Manager SHALL set the window icon
2. IF the icon file is not found, THEN THE Window Manager SHALL use the default system icon
3. THE Window Manager SHALL support both X11 and Wayland display servers

### Requirement 5: Startup Registration

**User Story:** As a Linux user, I want TokenTally to start automatically when I log in, so that the service is always available.

#### Acceptance Criteria

1. WHEN the `--install` flag is provided, THE Startup Manager SHALL register TokenTally for auto-start
2. WHEN the `--uninstall` flag is provided, THE Startup Manager SHALL remove the auto-start registration
3. THE Startup Manager SHALL support both systemd user services and desktop autostart entries
4. IF neither systemd user services nor desktop autostart are available, THEN THE Startup Manager SHALL log a warning and continue

### Requirement 6: Platform Detection

**User Story:** As a developer, I want the application to detect the Linux platform at runtime, so that platform-specific behavior can be enabled.

#### Acceptance Criteria

1. WHEN the application starts, THE Platform Detector SHALL identify the operating system as Linux
2. IF the platform is Linux, THEN THE Platform Detector SHALL return true for Linux platform checks
3. THE Platform Detector SHALL use standard Go runtime detection methods

### Requirement 7: Build Configuration

**User Story:** As a developer, I want to configure the build for Linux, so that the application can be built for Linux targets.

#### Acceptance Criteria

1. WHEN building for Linux, THE Build System SHALL include the Linux-specific source files
2. WHEN building for Linux, THE Build System SHALL NOT include Windows or macOS-specific source files
3. THE Build System SHALL support building for Linux AMD64, ARM64, and ARMv7 architectures

### Requirement 8: Testing Strategy

**User Story:** As a developer, I want to test Linux-specific functionality, so that the implementation is reliable.

#### Acceptance Criteria

1. WHEN running tests on Linux, THE Test Framework SHALL execute Linux-specific unit tests
2. WHEN running tests on Linux, THE Test Framework SHALL execute integration tests for tray icon functionality
3. WHEN running tests on Linux, THE Test Framework SHALL execute integration tests for service management
4. IF running tests on a non-Linux platform, THE Test Framework SHALL skip Linux-specific tests with a clear message

## Limitations and Considerations

### Platform Variations

- **Desktop Environments**: Linux has many desktop environments (GNOME, KDE, XFCE, etc.) with varying levels of tray icon support. The implementation should gracefully handle environments without tray support.
- **Display Servers**: Both X11 and Wayland are in use. The implementation should work with both, with X11 being the primary target due to broader systray support.
- **Service Managers**: While systemd is dominant, some distributions use alternative init systems. The service installation should detect systemd availability.

### Security Considerations

- **Service Installation**: Installing system services requires root/administrative privileges. The application should clearly indicate when elevated privileges are needed.
- **User Services**: For non-root installation, systemd user services should be supported as an alternative.

### User Experience

- **Tray Icon Visibility**: Some Linux distributions hide tray icons by default or require configuration. The application should provide clear instructions if tray icons are not visible.
- **Service Management**: Users should be able to manage the service using standard tools like `systemctl`.

## Testing Strategy

### Unit Tests

- Test platform detection logic
- Test service installation and uninstallation (using mocks for systemd)
- Test tray icon creation and menu handling (using mocks for system APIs)

### Integration Tests

- Test full application startup on Linux
- Test service installation and management via systemctl
- Test tray icon visibility and functionality
- Test window icon display on X11 and Wayland

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
