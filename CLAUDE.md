# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go Game Vision is a cross-platform Go tool framework providing process management, screen capture, image processing, and mouse simulation functionality for Windows and macOS. Designed specifically for other projects or programs to call as a library. It uses a platform-specific implementation pattern with build tags to support different operating systems.

## Development Commands

```bash
# Install dependencies
go mod tidy

# Run all tests
go test ./tests/...

# Run specific test package
go test ./tests/ -run TestProcessManager

# Run tests with verbose output
go test -v ./tests/...

# Build the project
go build .
```

## Architecture Overview

The project follows a clean architecture pattern with platform-specific implementations:

### Core Packages Structure
- `pkg/process/` - Process management with exact/fuzzy matching
- `pkg/capture/` - Screen capture functionality using platform APIs
- `pkg/image/` - Image comparison using GoCV and OpenCV
- `pkg/mouse/` - Mouse simulation with background clicking
- `pkg/utils/` - Error handling and logging utilities
- `tests/` - Test files for all packages

### Platform-Specific Implementation Pattern
Each major package uses build tags for platform-specific code:
- `*.go` - Cross-platform interfaces and types
- `*_windows.go` - Windows-specific implementations using Win32 API
- `*_darwin.go` - macOS-specific implementations: `ps` subprocesses in `pkg/process/`,
  cgo/Objective-C in `pkg/capture/` (CoreGraphics/Foundation/AppKit) and `pkg/mouse/`
  (CoreGraphics/CoreFoundation)

### cgo and Cross-Compilation Constraint
`pkg/capture/capture_darwin.go` and `pkg/mouse/mouse_darwin.go` are cgo files
(`#cgo CFLAGS: -x objective-c`). Cross-compiling from another OS defaults to
`CGO_ENABLED=0`, and **Go silently excludes every file that does `import "C"`
when cgo is off — a satisfied `//go:build darwin` tag does not save it.** The
symptom is a confusing "undefined" error pointing at the cross-platform file:

```
$ GOOS=darwin CGO_ENABLED=0 go vet ./pkg/capture/ ./pkg/mouse/
vet: pkg/capture/capture.go:75:9: undefined: newPlatformCapture
vet: pkg/mouse/mouse.go:72:9: undefined: newPlatformMouseClicker
```

Consequences to respect:
- Only `pkg/process/` and `pkg/utils/` can be vetted/built for darwin from a
  non-macOS host. `pkg/capture/` and `pkg/mouse/` require a real macOS toolchain
  (macOS SDK + frameworks), so CI vets them in the macOS job via `go vet ./...`.
- Do not "helpfully" add `capture`/`mouse` back into the Linux cross-compile step
  in `.github/workflows/ci.yml` — that is precisely what breaks the lint job.
- Windows implementations are pure `syscall`/`golang.org/x/sys` with no cgo, so
  all four non-gocv packages do cross-compile for `GOOS=windows` from any host.

### Key Interfaces
- `ProcessManager` - Process discovery and management interface
- `ScreenCapture` - Window capture interface
- `MouseClicker` - Mouse simulation interface

## Dependencies
- **GoCV (gocv.io/x/gocv)** - OpenCV bindings for image processing
- **golang.org/x/sys** - System calls for cross-platform functionality

## External Requirements
- **OpenCV** must be installed on the system
  - Windows: vcpkg or pre-compiled binaries
  - macOS: `brew install opencv`

## Platform-Specific Notes

### Windows
- Requires admin rights for certain system processes
- Uses BitBlt/PrintWindow APIs for window capture
- Can capture occluded windows
- Mouse simulation uses Windows API for background clicks

### macOS  
- Requires Screen Recording permissions
- Uses screencapture command for screen capture
- Mouse simulation uses Core Graphics for background clicks
- Process management uses ps command for reliable cross-platform compatibility

## Testing Strategy
Tests are organized by package functionality in the `tests/` directory, covering process management, screen capture, image comparison, and mouse simulation features. Tests include both unit tests and integration tests with appropriate platform-specific handling.