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

Perception and input (low level):
- `pkg/platform/` - Process-wide OS setup that must precede any window/input call.
  Currently per-monitor-v2 DPI awareness on Windows. Both `capture` and `mouse`
  depend on it; it exists as its own leaf package because the DPI opt-in is
  process-wide and one-shot, and living in `mouse`'s init meant a program
  importing only `capture` silently read DPI-virtualised window rectangles.
- `pkg/process/` - Process management with exact/fuzzy matching
- `pkg/capture/` - Window capture. Stateless `ScreenCapture` for one-off shots,
  plus `FrameCapturer` (a session that reuses its pixel buffer) for loops, and
  `FindWindows`/`FindWindowByTitle` for title-first lookup.
- `pkg/image/` - Template matching (`Matcher` + `Template` = the hot path) and
  HSV colour detection (`ColorDetector`), both via GoCV/OpenCV
- `pkg/mouse/` - Clicks, drag, scroll
- `pkg/keyboard/` - Synthetic keyboard input, positional key codes
- `pkg/utils/` - Error handling and logging utilities

Decision and safety (high level):
- `pkg/safety/` - The emergency stop. Not optional for anything that moves the
  real cursor: macOS has no true background click, so a running script owns the
  mouse and the user cannot reach a stop button.
- `pkg/automation/` - `Session` (capture+match+input+templates wired together,
  with `WaitFor`/`WaitVanish`/`WaitStable`) and `Machine` (the
  recognise → decide → act loop over declared states, with unknown-screen and
  stuck-state fallbacks)

- `tests/` - Test files for all packages

### The coordinate-space contract (read before touching click math)

Captured images are in **physical pixels**; `WindowInfo.Rect` and the coordinates
`mouse.BackgroundClick` accepts are in the platform's **click space** (logical
points on macOS, physical pixels on Windows). `WindowInfo.ScaleFactor` is the
ratio, so a match at pixel `(px,py)` maps to `Rect.Min + (px,py)/ScaleFactor`.

Use `WindowInfo.PixelToClickPoint` or `MatchResult.ToScreenCenter`; never add a
pixel offset straight onto `Rect.Min`. Doing so was a real bug: on a 2880x1864
Retina display a match at the centre of a 1073pt-wide window resolved to
(1435,1122) instead of (830,573). `tests/scale_coords_test.go` pins this.

Related trap: `screencapture -l<id>` includes the window's drop shadow unless
`-o` is passed, inflating the frame by ~34pt per side so that the image origin no
longer coincides with the window origin (measured 2.127x instead of 2.0x).

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
- Only `pkg/process/`, `pkg/utils/` and `pkg/platform/` can be vetted/built for
  darwin from a non-macOS host. `pkg/capture/`, `pkg/mouse/` and `pkg/keyboard/`
  require a real macOS toolchain (macOS SDK + frameworks), so CI vets them in the
  macOS job via `go vet ./...`.
- Do not "helpfully" add `capture`/`mouse`/`keyboard` back into the Linux
  cross-compile step in `.github/workflows/ci.yml` — that is precisely what
  breaks the lint job.
- Windows implementations are pure `syscall`/`golang.org/x/sys` with no cgo, so
  every non-gocv package cross-compiles for `GOOS=windows` from any host —
  including `pkg/safety`, which only depends on `mouse` and `utils`.
- **`pkg/image/` and `pkg/automation/` cannot be cross-compiled at all**, because
  gocv needs cgo plus a platform-native OpenCV. They are covered only by the
  macOS job. Adding them to a `GOOS=windows` step fails in `runtime/cgo` with
  `'windows.h' file not found`.

### Build constraints in .m files are load-bearing

`pkg/capture/sck_capture.m` and `pkg/capture/window_query.m` each start with
`//go:build darwin`. Build constraints in `.m` files *are* honoured by the go
tool, and without them a `GOOS=windows` build of `pkg/capture` fails outright:

```
package .../pkg/capture: Objective-C source files not allowed when not using cgo or SWIG: sck_capture.m
```

The Windows build excludes every cgo `.go` file, so the package stops being a cgo
package while the `.m` file is still sitting in the directory. Do not remove those
constraints.

### Key Interfaces
- `ProcessManager` - Process discovery and management interface
- `ScreenCapture` - Window capture interface
- `MouseClicker` - Mouse simulation interface

## Dependencies
- **GoCV (gocv.io/x/gocv)** - OpenCV bindings for image processing
- **golang.org/x/sys** - System calls for cross-platform functionality

## External Requirements
- **OpenCV 4.x** must be installed on the system. gocv wraps the OpenCV 4 C++ API
  and does **not** support OpenCV 5 — the failure mode is
  `Package 'opencv4' not found` from `pkg-config`.
  - Windows: vcpkg or pre-compiled binaries
  - macOS: `brew install opencv@4` (the main `opencv` formula is 5.0.0 now and will
    not work). It is keg-only, so export the pkgconfig dir before building:
    `export PKG_CONFIG_PATH="$(brew --prefix opencv@4)/lib/pkgconfig"`

## Platform-Specific Notes

### Windows
- Requires admin rights for certain system processes
- Uses BitBlt/PrintWindow APIs for window capture
- Can capture occluded windows
- Mouse simulation uses Windows API for background clicks

### macOS
- Requires Screen Recording permission. Without it, ScreenCaptureKit returns no
  windows and `kCGWindowName` comes back empty, so titles cannot be matched.
  Keyboard/mouse injection additionally needs Accessibility permission.
- Capture uses **ScreenCaptureKit** (`SCScreenshotManager`, macOS 14+), which
  returns an in-memory CGImage. The old `screencapture` subprocess remains only as
  a fallback in `captureWindowViaSubprocess`: it forks a process, encodes a PNG and
  does disk IO, measuring 130-190ms/frame (5-7 fps) against ~50ms (20 fps) for SCK.
  `FrameCapturer` is SCK-only by design — falling back to a 150ms/frame path in a
  loop would be worse than failing.
- **There is no true background click.** `CGEventPost` drives the real system
  cursor, so a running script owns the mouse; `PostMessageClick` and
  `PostKeyToWindow` are deliberately unimplemented (Win32-only concepts). Design
  automation for the foreground-exclusive model and always arm `pkg/safety`.
- **Target windows by title, not by process name.** Many Mac games are Windows
  builds under Wine/CrossOver/Whisky, where `ps` shows `wine64-preloader` and the
  game's name appears only in the window title; Steam-distributed games often have
  a launcher parent that matches the name while the window belongs to a child. Use
  `capture.FindWindowByTitle` / `automation.NewSessionByTitle`.
- Process management uses ps command for reliable cross-platform compatibility

### Performance notes (measured on this project)

ROI is the single biggest lever, and multi-scale is the biggest waste:

| operation (1920x1050 frame) | time | fps |
|---|---|---|
| old API (`CaptureWindowByPID` + `MultiScaleTemplateMatch`) | 1.31s | 0.8 |
| hot path, full-frame match | 158ms | 6.3 |
| hot path + 300x200 ROI | 39ms | 25.7 |
| match only, full frame | 120ms | 8.4 |
| match only, 300x200 ROI | 2.0ms | 496 |
| match only, 11 scales | 925ms | 1.1 |

So: always set `MatchOptions.ROI`, and leave `MatchOptions.Scales` empty unless the
target genuinely changes size — a game window's resolution is fixed, so scanning
scales costs one full match per entry for nothing. `RawFrame` carries native BGRA
precisely so the hot path can hand it to OpenCV without a per-pixel byte swap.

Remaining bottleneck after ROI is the capture itself (~37ms). Pushing the ROI down
into `SCStreamConfiguration.sourceRect` would help, but was left undone: it makes
the frame's coordinate origin no longer the window origin, and this project has
already been bitten once by exactly that class of bug.

## Testing Strategy
Tests are organized by package functionality in the `tests/` directory, covering process management, screen capture, image comparison, and mouse simulation features. Tests include both unit tests and integration tests with appropriate platform-specific handling.