# Go Game Vision

English | [中文](README.md)

A cross-platform Go project that implements program window capture functionality for Windows and macOS, including process management, window screenshot, image processing, and OCR text recognition modules.

## Features

### 🔍 Process Management Module
- Get process PID by program name
- Support both fuzzy matching and exact matching modes
- Handle multiple processes with the same name
- Cross-platform compatibility (Windows/macOS)

### 📸 Window Screenshot Module
- **Windows Platform**: Uses Windows API (BitBlt/PrintWindow) for window screenshots
- **macOS Platform**: Uses system commands and AppleScript for window screenshots
- **Key Feature**: On Windows, can capture windows even when they are obscured by other windows
- Support multiple image format outputs (PNG, JPEG, BMP, GIF)
- Provides methods to capture program window screenshots by PID

### 🖼️ Image Processing Module
- Integrates GoCV library for image comparison functionality
- Provides image similarity calculation methods
- Supports multiple comparison algorithms:
  - Template Matching
  - Feature Matching
  - Histogram Comparison
  - Structural Similarity

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
# Install Tesseract OCR
winget install UB-Mannheim.TesseractOCR
```

#### macOS
```bash
# Install using Homebrew
brew install opencv tesseract
```

## Quick Start

### Command Line Usage

```bash
# List all processes
go run main.go list

# Capture window of specified process
go run main.go capture notepad

# Capture window and specify output filename
go run main.go capture explorer window.png

# Show help information
go run main.go help
```

### Programming Interface Usage

```go
package main

import (
    "fmt"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
    "github.com/lnatpunblhna/go-game-vision/pkg/image"
    "github.com/lnatpunblhna/go-game-vision/pkg/ocr"
)

func main() {
    // 1. Process Management
    pid, err := process.GetProcessPIDByName("notepad", process.FuzzyMatch)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Found notepad process, PID: %d\n", pid)

    // 2. Window Screenshot
    img, err := capture.CaptureWindowByPID(pid, capture.DefaultCaptureOptions())
    if err != nil {
        panic(err)
    }

    // 3. Save Screenshot
    err = capture.CaptureAndSave(pid, "notepad.png", capture.PNG, 90)
    if err != nil {
        panic(err)
    }

    // 4. Image Comparison
    similarity, err := image.CalculateSimilarity(img1, img2)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Image similarity: %.2f\n", similarity)

    // 5. OCR Text Recognition
    text, err := ocr.RecognizeTextFromFile("notepad.png", ocr.English)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Recognized text: %s\n", text)
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

// Capture window
options := capture.DefaultCaptureOptions()
img, err := capturer.CaptureWindowByPID(pid, options)

// Capture screen
img, err := capturer.CaptureScreen(options)

// Save image
err = capturer.SaveImage(img, "output.png", capture.PNG, 90)
```

### Image Processing (pkg/image)

```go
// Create image comparer
comparer := image.NewImageComparer(image.TemplateMatching)

// Compare images
result, err := comparer.CompareImages(img1, img2)
fmt.Printf("Similarity: %.2f, Location: (%d, %d)\n", 
    result.Similarity, result.Location.X, result.Location.Y)
```



## Project Structure

```
go-game-vision/
├── pkg/                    # Core packages
│   ├── process/           # Process management
│   │   ├── process.go     # Cross-platform interface
│   │   ├── process_windows.go  # Windows implementation
│   │   └── process_darwin.go   # macOS implementation
│   ├── capture/           # Window screenshot
│   │   ├── capture.go     # Cross-platform interface
│   │   ├── capture_windows.go  # Windows implementation
│   │   └── capture_darwin.go   # macOS implementation
│   ├── image/             # Image processing
│   │   └── compare.go     # Image comparison functionality
│   ├── ocr/               # OCR recognition
│   │   └── ocr.go         # OCR functionality
│   └── utils/             # Utility modules
│       ├── logger.go      # Logging
│       └── errors.go      # Error handling
├── tests/                 # Test files
│   ├── process_test.go    # Process management tests
│   └── capture_test.go    # Screenshot functionality tests
├── main.go               # Main program
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

### macOS Platform
- Screen recording permission required
- Some system processes may not be capturable
- Using AppleScript to get window information may require accessibility permissions

### Performance Optimization
- Reuse screenshot capturer instances for bulk screenshot operations
- OCR recognition is time-consuming, recommend executing in background threads
- Image comparison performance depends on image size and algorithm choice

## Contributing

1. Fork the project
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.