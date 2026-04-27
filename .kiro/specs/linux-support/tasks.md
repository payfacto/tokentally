# Implementation Plan: Linux Support for TokenTally

## Overview

This implementation adds Linux support to TokenTally by creating platform-specific files for tray icon, service management, window icon, and startup registration. The implementation follows the existing platform-specific code patterns using Go build tags.

## Tasks

- [x] 1. Create platform-specific build configuration
  - Create `.kiro/specs/linux-support/.config.kiro` with the required spec configuration
  - _Requirements: 1.1, 1.2, 1.3, 7.1, 7.2, 7.3_

- [x] 2. Implement tray icon support for Linux
  - [x] 2.1 Create `app/tray_linux.go` with systray integration
    - Implement `StartTray()` method using `github.com/getlantern/systray`
    - Create tray icon with tooltip "TokenTally"
    - Add menu items: "Open Dashboard", "Scan Now", "Quit TokenTally"
    - Handle menu item clicks for each action
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_
  
  - [x] 2.2 Write property test for tray icon creation
    - **Property 2: Tray icon creation**
    - **Validates: Requirements 2.1, 2.2**
    - Test that tray icon is created with correct tooltip
    - _Requirements: 2.1, 2.2_
  
  - [x] 2.3 Write property test for tray menu functionality
    - **Property 3: Tray menu functionality**
    - **Validates: Requirements 2.3, 2.4, 2.5**
    - Test that menu item clicks trigger correct actions
    - _Requirements: 2.3, 2.4, 2.5_
  
  - [x] 2.4 Write property test for graceful tray degradation
    - **Property 4: Graceful tray degradation**
    - **Validates: Requirement 2.6**
    - Test that application continues when tray is not supported
    - _Requirements: 2.6_

- [x] 3. Implement service management for Linux
  - [x] 3.1 Create `app/service_linux.go` with systemd integration
    - Implement `GetServiceStatus()` to check service state
    - Implement `InstallService()` to register systemd service
    - Implement `UninstallService()` to remove systemd service
    - Implement `RunService()` to run as systemd-managed service
    - Use `github.com/godbus/dbus/v5` for D-Bus communication
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_
  
  - [x] 3.2 Write property test for service installation
    - **Property 5: Service installation**
    - **Validates: Requirement 3.1**
    - Test that service is registered with correct name
    - _Requirements: 3.1_
  
  - [x] 3.3 Write property test for service uninstallation
    - **Property 6: Service uninstallation**
    - **Validates: Requirement 3.2**
    - Test that service is removed correctly
    - _Requirements: 3.2_
  
  - [x] 3.4 Write property test for service execution
    - **Property 7: Service execution**
    - **Validates: Requirements 3.3, 3.4**
    - Test that service runs with 30-second scan interval
    - _Requirements: 3.3, 3.4_
  
  - [x] 3.5 Write property test for service error handling
    - **Property 8: Service error handling**
    - **Validates: Requirement 3.5**
    - Test that errors are logged and non-zero exit occurs
    - _Requirements: 3.5_
  
  - [x] 3.6 Write property test for service name consistency
    - **Property 9: Service name consistency**
    - **Validates: Requirement 3.6**
    - Test that service name is "tokentally" on all platforms
    - _Requirements: 3.6_

- [x] 4. Implement window icon support for Linux
  - [x] 4.1 Create `app/wndicon_linux.go` with X11/Wayland support
    - Implement `SetWindowIcon()` method
    - Load icon from `assets/icon.png`
    - Support both X11 and Wayland display servers
    - Use default system icon if file not found
    - _Requirements: 4.1, 4.2, 4.3_
  
  - [x] 4.2 Write property test for window icon setting
    - **Property 10: Window icon setting**
    - **Validates: Requirement 4.1**
    - Test that window icon is set when window is created
    - _Requirements: 4.1_
  
  - [x] 4.3 Write property test for window icon fallback
    - **Property 11: Window icon fallback**
    - **Validates: Requirement 4.2**
    - Test that default system icon is used when file is missing
    - _Requirements: 4.2_
  
  - [x] 4.4 Write property test for display server compatibility
    - **Property 12: Display server compatibility**
    - **Validates: Requirement 4.3**
    - Test that icon works on both X11 and Wayland
    - _Requirements: 4.3_

- [x] 5. Create Linux entry point
  - [x] 5.1 Create `main_linux.go` with platform-specific initialization
    - Implement `main()` function with build tag `//go:build linux`
    - Handle `--install`, `--uninstall`, and `--service` flags
    - Implement `runInstall()` for startup registration
    - Implement `runUninstall()` for startup removal
    - Implement `runService()` for service mode
    - Implement `runUI()` for UI mode with tray support
    - _Requirements: 1.1, 1.2, 3.1, 3.2, 5.1, 5.2_

- [x] 6. Implement startup registration for Linux
  - [x] 6.1 Create `app/startup_linux.go` with systemd user service support
    - Implement `InstallStartup()` to register systemd user service
    - Implement `UninstallStartup()` to remove systemd user service
    - Support desktop autostart entries as fallback
    - Log warning if neither mechanism is available
    - _Requirements: 5.1, 5.2, 5.3, 5.4_
  
  - [x] 6.2 Write property test for startup registration
    - **Property 13: Startup registration**
    - **Validates: Requirement 5.1**
    - Test that startup registration works with systemd user service
    - _Requirements: 5.1_
  
  - [x] 6.3 Write property test for startup removal
    - **Property 14: Startup removal**
    - **Validates: Requirement 5.2**
    - Test that startup removal works correctly
    - _Requirements: 5.2_
  
  - [x] 6.4 Write property test for startup mechanism support
    - **Property 15: Startup mechanism support**
    - **Validates: Requirement 5.3**
    - Test that both systemd user service and desktop autostart are supported
    - _Requirements: 5.3_
  
  - [x] 6.5 Write property test for startup fallback
    - **Property 16: Startup fallback**
    - **Validates: Requirement 5.4**
    - Test that application continues when neither mechanism is available
    - _Requirements: 5.4_

- [x] 7. Implement platform detection
  - [x] 7.1 Create `app/platform.go` with platform detection functions
    - Implement `IsLinux()` to detect Linux platform
    - Implement `IsWindows()` to detect Windows platform
    - Implement `IsDarwin()` to detect macOS platform
    - Implement `GetPlatformName()` to return current OS name
    - Implement `GetArchitecture()` to return current architecture
    - _Requirements: 6.1, 6.2, 6.3_

- [x] 8. Update existing files for Linux compatibility
  - [x] 8.1 Update `app/app.go` to include Linux tray interface
    - Ensure `StartTray()` method is available for Linux build
    - Ensure `SetWindowIcon()` method is available for Linux build
    - Ensure service methods are available for Linux build
    - _Requirements: 2.1, 4.1, 3.1_
  
  - [x] 8.2 Update `main_shared.go` to include Linux platform detection
    - Ensure platform detection works for Linux builds
    - _Requirements: 6.1, 6.2_

- [x] 9. Create integration tests for Linux
  - [x] 9.1 Create `integration/linux_test.go` with integration tests
    - Test full application startup on Linux
    - Test tray icon visibility and functionality
    - Test service installation and management
    - Skip tests on non-Linux platforms with clear message
    - _Requirements: 8.1, 8.2, 8.3, 8.4_
  
  - [x] 9.2 Write property test for test framework execution
    - **Property 21: Test framework execution**
    - **Validates: Requirements 8.1, 8.4**
    - Test that Linux-specific tests run on Linux and skip on other platforms
    - _Requirements: 8.1, 8.4_
  
  - [x] 9.3 Write property test for integration test execution
    - **Property 22: Integration test execution**
    - **Validates: Requirements 8.2, 8.3**
    - Test that integration tests for tray and service run on Linux
    - _Requirements: 8.2, 8.3_

- [x] 10. Checkpoint - Ensure all tests pass
  - All tests pass successfully.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties
- Unit tests validate specific examples and edge cases
- All Linux-specific files use `//go:build linux` build tag
- Implementation follows existing platform-specific patterns in the codebase
- Uses `github.com/getlantern/systray` for tray icon (same as Windows)
- Uses `github.com/godbus/dbus/v5` for D-Bus communication with systemd
