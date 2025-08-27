# Go Game Vision

English | [中文](README.md)

A cross-platform Go tool framework providing process management, screen capture, image processing, and mouse simulation functionality for Windows and macOS. Designed specifically for other projects or programs to call as a library.

## Features

### 🔍 Process Management Module
- Get process PID by program name
- Support both fuzzy matching and exact matching modes
- Handle multiple processes with the same name
- Cross-platform compatibility (Windows/macOS)

### 📸 Screen Capture Module
- **Windows Platform**: Uses Windows API (BitBlt/PrintWindow) for window screenshots
- **macOS Platform**: Uses Core Graphics API and screencapture command for true window capture
- **Key Features**:
  - Windows: Can capture windows even when they are obscured by other windows
  - macOS: Can capture specific windows by process ID (even when obscured)
  - Automatically handles multi-process applications (like Safari, Chrome, etc.)
- Support multiple image format outputs (PNG, JPEG, BMP, GIF)
- Provides window information retrieval (position, size, state, etc.)
- Convenient capture and save methods

### 🖼️ Image Processing Module
- Integrates GoCV library for image comparison functionality
- Provides image similarity calculation methods
- Supports multiple comparison algorithms:
  - Template Matching
  - Feature Matching
  - Histogram Comparison
  - Structural Similarity

### 🖱️ Mouse Simulation Module
- Cross-platform background mouse clicking functionality
- Supports left, right, and middle button clicks
- Background clicking without moving the mouse cursor
- Screen coordinate validation and boundary checking
- Configurable click delay settings

## System Requirements

### Basic Requirements
- Go 1.19 or higher
- Windows 10+ or macOS 10.14+

### Dependencies
- [GoCV](https://gocv.io/) - Go bindings for OpenCV (for image processing)
- golang.org/x/sys - System call support

### External Dependencies
- **OpenCV**: Required for image processing functionality

## Installation Guide

### 1. Clone the Project
```bash
git clone https://github.com/lnatpunblhna/go-game-vision.git
cd go-game-vision
```

### 2. Install Go Dependencies
```bash
go mod tidy
```

### 3. Install External Dependencies

#### Windows
```bash
# Install OpenCV (using vcpkg or pre-compiled version)
```

#### macOS
```bash
# Install using Homebrew
brew install opencv
```

## Quick Start

### Programming Interface Usage

```go
package main

import (
    "fmt"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
    "github.com/lnatpunblhna/go-game-vision/pkg/image"
    "github.com/lnatpunblhna/go-game-vision/pkg/mouse"
)

func main() {
    // 1. Process Management
    pid, err := process.GetProcessPIDByName("notepad", process.FuzzyMatch)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Found notepad process, PID: %d\n", pid)

    // 2. Get Window Information
    windowInfo, err := capture.GetWindowInfoByPID(pid)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Window size: %dx%d\n", windowInfo.Rect.Dx(), windowInfo.Rect.Dy())

    // 3. Capture Window (even when obscured)
    img, err := capture.CaptureWindowByPID(pid, capture.DefaultCaptureOptions())
    if err != nil {
        panic(err)
    }

    // 4. Save Screenshot
    err = capture.CaptureAndSave(pid, "window_capture.png", capture.PNG, 90)
    if err != nil {
        panic(err)
    }

    // 5. Image Comparison
    img1, _ := image.LoadImage("image1.png")
    img2, _ := image.LoadImage("image2.png")
    similarity, err := image.CalculateSimilarity(img1, img2)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Image similarity: %.2f\n", similarity)

    // 6. Mouse Simulation Click (within window coordinates)
    clickX := windowInfo.Rect.Min.X + 100 // Relative position within window
    clickY := windowInfo.Rect.Min.Y + 100
    err = mouse.BackgroundLeftClick(clickX, clickY)
    if err != nil {
        panic(err)
    }
    fmt.Println("Background click completed")
}
```

## API Documentation

### Process Management (pkg/process)

```go
// Get process PID
pid, err := process.GetProcessPIDByName("program_name", process.ExactMatch)

// Get all matching PIDs
pids, err := process.GetAllProcessPIDsByName("program_name", process.FuzzyMatch)

// Create process manager
manager := process.NewProcessManager()
processes, err := manager.ListAllProcesses()
```

### Window Screenshot (pkg/capture)

```go
// Create screenshot capturer
capturer := capture.NewScreenCapture()

// Capture specific process window (even when obscured)
options := capture.DefaultCaptureOptions()
img, err := capturer.CaptureWindowByPID(pid, options)

// Get window information
windowInfo, err := capturer.GetWindowInfoByPID(pid)
fmt.Printf("Window position: (%d, %d), size: %dx%d\n", 
    windowInfo.Rect.Min.X, windowInfo.Rect.Min.Y,
    windowInfo.Rect.Dx(), windowInfo.Rect.Dy())

// Save image
err = capturer.SaveImage(img, "output.png", capture.PNG, 90)

// Convenience function: capture and save directly
err = capture.CaptureAndSave(pid, "window.png", capture.PNG, 90)

// Convenience function: get window information
windowInfo, err := capture.GetWindowInfoByPID(pid)
```

### Image Processing (pkg/image)

```go
// Create image comparer
comparer := image.NewImageComparer(image.TemplateMatching)

// Compare images
result, err := comparer.CompareImages(img1, img2)
fmt.Printf("Similarity: %.2f, Location: (%d, %d)\n", 
    result.Similarity, result.Location.X, result.Location.Y)

// Load image file
img, err := image.LoadImage("example.png")

// Convenience function to calculate similarity
similarity, err := image.CalculateSimilarity(img1, img2)
```

### Mouse Simulation (pkg/mouse)

```go
// Create mouse controller
clicker := mouse.NewMouseClicker()

// Background click (without moving cursor)
options := mouse.DefaultClickOptions()
err := clicker.BackgroundClick(100, 100, options)

// Convenience functions
err = mouse.BackgroundLeftClick(100, 100)    // Left click
err = mouse.BackgroundRightClick(100, 100)   // Right click
err = mouse.BackgroundMiddleClick(100, 100)  // Middle click

// Coordinate validation
err = mouse.ValidateCoordinates(100, 100)

// Get screen size
width, height, err := clicker.GetScreenSize()
```



## Project Structure

```
go-game-vision/
├── pkg/                    # Core packages
│   ├── process/           # Process management
│   │   ├── process.go     # Cross-platform interface
│   │   ├── process_windows.go  # Windows implementation
│   │   └── process_darwin.go   # macOS implementation
│   ├── capture/           # Screen capture
│   │   ├── capture.go     # Cross-platform interface
│   │   ├── capture_windows.go  # Windows implementation
│   │   └── capture_darwin.go   # macOS implementation
│   ├── image/             # Image processing
│   │   └── compare.go     # Image comparison functionality
│   ├── mouse/             # Mouse simulation
│   │   ├── mouse.go       # Cross-platform interface
│   │   ├── mouse_windows.go    # Windows implementation
│   │   └── mouse_darwin.go     # macOS implementation
│   └── utils/             # Utility modules
│       ├── logger.go      # Logging
│       └── errors.go      # Error handling
├── tests/                 # Test files
│   ├── process_test.go    # Process management tests
│   ├── capture_test.go    # Screenshot functionality tests
│   ├── image_compare_test.go  # Image comparison tests
│   └── mouse_test.go      # Mouse simulation tests
├── go.mod                # Go module file
├── README.md             # Project documentation (Chinese)
└── README_EN.md          # Project documentation (English)
```

## Running Tests

```bash
# Run all tests
go test ./tests/...

# Run specific tests
go test ./tests/ -run TestProcessManager

# Run tests with verbose output
go test -v ./tests/...
```

## Notes

### Windows Platform
- Administrator privileges may be required to capture windows of certain system processes
- Using PrintWindow API can capture obscured windows
- Supports DPI awareness
- Mouse simulation uses Windows API for background clicks

### macOS Platform
- Screen recording permission required
- Some system processes may not be capturable
- Mouse simulation uses Core Graphics for background clicks
- Process management uses ps command for reliable cross-platform compatibility

### Performance Optimization
- Reuse screenshot capturer instances for bulk screenshot operations
- Image comparison performance depends on image size and algorithm choice
- Mouse simulation operations should include appropriate delays to avoid excessive frequency

## Contributing

1. Fork the project
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.