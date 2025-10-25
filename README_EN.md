# Go Game Vision

<div align="center">

English | [中文](README.md)

A powerful cross-platform Go automation framework providing process management, screen capture, image recognition, and intelligent mouse simulation for Windows and macOS.

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.19-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS-lightgrey)](https://github.com/lnatpunblhna/go-game-vision)

</div>

---

## ✨ Core Features

### 🔍 Process Management Module
- ✅ Quick process PID retrieval by program name
- ✅ Support for both fuzzy and exact matching modes
- ✅ Intelligent handling of multiple processes with the same name
- ✅ Cross-platform compatibility (Windows/macOS)

### 📸 Screen Capture Module
- ✅ **Windows**: Uses BitBlt/PrintWindow API, can capture even when obscured
- ✅ **macOS**: Uses Core Graphics API, supports specific window capture
- ✅ Automatic handling of multi-process applications (Chrome, Safari, etc.)
- ✅ Multiple format support (PNG, JPEG, BMP, GIF)
- ✅ Complete window information (position, size, state)

### 🖼️ Image Recognition Module
- ✅ **Multi-Scale Template Matching** - Automatically adapts to window scaling and DPI changes
- ✅ **Smart Coordinate System** - Window-relative coordinates ⇄ Screen absolute coordinates automatic conversion
- ✅ **One-Stop Match & Click** - Execute clicks directly after image recognition
- ✅ Multiple comparison algorithms:
  - Template Matching
  - Feature Matching
  - Histogram Comparison
  - Structural Similarity
  - **Multi-Scale Template Matching**⭐

### 🖱️ Intelligent Mouse Simulation Module
- ✅ **Anti-Cheat Click** - Through hardware input queue, difficult to detect by games
- ✅ **True Background Click** - PostMessage/SendMessage approach
- ✅ **Random Delays** - Simulates human operations (5-15ms random delays)
- ✅ **Automatic Focus Restoration** - Automatically restores original window focus after clicking
- ✅ **Smart Child Window Finding** - Automatically locates actual rendering window
- ✅ Support for left, right, and middle button clicks
- ✅ Screen coordinate validation and boundary checking

---

## 📦 Installation

### Prerequisites
- **Go**: 1.19 or higher
- **System**: Windows 10+ or macOS 10.14+
- **OpenCV**: Required for image processing features

### Quick Install

```bash
go get github.com/lnatpunblhna/go-game-vision
```

### OpenCV Installation

#### Windows
```bash
# Method 1: Using vcpkg
vcpkg install opencv4

# Method 2: Download pre-compiled version
# Visit https://github.com/hybridgroup/gocv#windows
```

#### macOS
```bash
brew install opencv
```

---

## 🚀 Quick Start

### Basic Example - Window Capture

```go
package main

import (
    "log"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func main() {
    // 1. Find process
    pid, err := process.GetProcessPIDByName("notepad.exe", process.ExactMatch)
    if err != nil {
        log.Fatal(err)
    }

    // 2. Capture and save
    err = capture.CaptureAndSave(pid, "window.png", capture.PNG, 90)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Screenshot successful!")
}
```

### Advanced Example - Multi-Scale Image Matching & Smart Click

```go
package main

import (
    "log"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/image"
    "github.com/lnatpunblhna/go-game-vision/pkg/mouse"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func main() {
    // 1. Find game process
    pid, _ := process.GetProcessPIDByName("game.exe", process.ExactMatch)

    // 2. Get window information
    windowInfo, _ := capture.GetWindowInfoByPID(pid)

    // 3. Capture game window
    windowImage, _ := capture.CaptureWindowByPID(pid, nil)

    // 4. Load button template image
    buttonTemplate, _ := image.LoadImage("button.png")

    // 5. Multi-scale matching (automatically adapts to window scaling)
    config := &image.MultiScaleConfig{
        MinScale:   0.7,  // Minimum 70%
        MaxScale:   1.3,  // Maximum 130%
        ScaleStep:  0.05, // Step 5%
        Threshold:  0.75, // Similarity threshold 75%
        MaxResults: 5,
    }

    result, err := image.MultiScaleTemplateMatch(windowImage, buttonTemplate, config)
    if err != nil {
        log.Fatal("Button not found:", err)
    }

    log.Printf("Button found! Similarity: %.2f%%", result.Similarity*100)

    // 6. Anti-cheat intelligent click
    screenCoords := result.ToScreenCoordinates(windowInfo)
    clicker := mouse.NewMouseClicker()

    clickOptions := &mouse.ClickOptions{
        Button:       mouse.LeftButton,
        Delay:        50,
        RandomDelay:  true,  // Add random delay to simulate human
        RestoreFocus: true,  // Restore focus after click
    }

    err = clicker.BackgroundClick(screenCoords.X, screenCoords.Y, clickOptions)
    if err != nil {
        log.Fatal("Click failed:", err)
    }

    log.Println("✓ Click successful!")
}
```

---

## 📖 Detailed Usage

### Process Management

```go
// Exact match
pid, err := process.GetProcessPIDByName("notepad.exe", process.ExactMatch)

// Fuzzy match
pid, err := process.GetProcessPIDByName("note", process.FuzzyMatch)

// Get all matching processes
pm := process.NewProcessManager()
procs, err := pm.GetProcessByName("chrome", process.FuzzyMatch)
for _, proc := range procs {
    fmt.Printf("PID: %d, Name: %s\n", proc.PID, proc.Name)
}
```

### Window Capture

```go
// Method 1: Convenience function
err := capture.CaptureAndSave(pid, "output.png", capture.PNG, 90)

// Method 2: Full control
capturer := capture.NewScreenCapture()
img, err := capturer.CaptureWindowByPID(pid, nil)

// Get window information
windowInfo, err := capture.GetWindowInfoByPID(pid)
fmt.Printf("Window: %s, Size: %dx%d\n",
    windowInfo.Title,
    windowInfo.Rect.Dx(),
    windowInfo.Rect.Dy())
```

### Image Matching

#### Basic Template Matching
```go
comparer := image.NewImageComparer(image.TemplateMatching)
result, err := comparer.CompareImages(sourceImage, templateImage)

fmt.Printf("Similarity: %.2f\n", result.Similarity)
fmt.Printf("Window coords: (%d, %d)\n", result.Location.X, result.Location.Y)
```

#### Multi-Scale Matching (Recommended)⭐
```go
config := &image.MultiScaleConfig{
    MinScale:   0.7,   // Minimum scale 70%
    MaxScale:   1.3,   // Maximum scale 130%
    ScaleStep:  0.05,  // Step 5%
    Threshold:  0.75,  // Threshold 75%
    MaxResults: 5,     // Return top 5 results
}

result, err := image.MultiScaleTemplateMatch(sourceImage, templateImage, config)

// Find all matches
results, err := image.MultiScaleTemplateMatchAll(sourceImage, templateImage, config)
for i, r := range results {
    fmt.Printf("[%d] Similarity: %.2f, Scale: %.0f%%\n",
        i+1, r.Similarity*100, r.Scale*100)
}
```

#### Coordinate Conversion
```go
// Window-relative coords → Screen absolute coords
screenCoords := result.ToScreenCoordinates(windowInfo)

// Bounding box conversion
screenBBox := result.ToScreenBoundingBox(windowInfo)
```

### Smart Clicking

#### Method 1: Anti-Cheat Click (Recommended for Games)⭐
```go
clicker := mouse.NewMouseClicker()

options := &mouse.ClickOptions{
    Button:       mouse.LeftButton,
    Delay:        50,              // Base delay 50ms
    RandomDelay:  true,            // Add 5-15ms random delay
    RestoreFocus: true,            // Auto restore original window focus
}

// Use SendInput (through hardware input queue, hard to detect)
err := clicker.BackgroundClick(x, y, options)
```

#### Method 2: True Background Click (May be detected by anti-cheat)
```go
// PostMessage method (doesn't activate window, but easily detected)
err := clicker.PostMessageClick(windowInfo.Handle, x, y, options)

// Auto find child window and click
if windowsClicker, ok := clicker.(*mouse.WindowsMouseClicker); ok {
    err = windowsClicker.PostMessageClickAtScreenCoords(
        windowInfo.Handle, screenX, screenY, options)
}
```

#### Method 3: One-Stop Match & Click
```go
// Click directly after image matching
result, err := image.FindAndLeftClick(
    sourceImage,
    templateImage,
    windowInfo,
    image.TemplateMatching)
```

---

## 🎯 Use Cases

### Game Automation
- ✅ Auto-click game buttons
- ✅ Auto-collect rewards
- ✅ Auto-battle/leveling
- ✅ Anti-cheat detection

### UI Automation Testing
- ✅ Desktop application automation testing
- ✅ Cross-platform UI testing
- ✅ Screenshot comparison testing

### Office Automation
- ✅ Batch window processing
- ✅ Automated click workflows
- ✅ Window monitoring

---

## 🛡️ Anti-Cheat Strategies

### Click Method Comparison

| Method | Principle | Window Activation | Detection Difficulty | Success Rate | Use Case |
|--------|-----------|-------------------|---------------------|-------------|-----------|
| **SendInput + Random Delay** ⭐ | Hardware input queue | ✅ Briefly | 🟢 Hard to detect | 🟢 High | Most games |
| **PostMessage/SendMessage** | Window message | ❌ No | 🔴 Easy to detect | 🔴 Low | Simple apps |
| **SendInput (No delay)** | Hardware input queue | ✅ Briefly | 🟡 Medium | 🟡 Medium | General apps |

### Recommended Configuration

#### Bypass Anti-Cheat (Recommended)⭐
```go
options := &mouse.ClickOptions{
    Button:       mouse.LeftButton,
    Delay:        50,
    RandomDelay:  true,   // ✅ Random delay 5-15ms
    RestoreFocus: true,   // ✅ Auto restore focus
}
```

#### Fully Background (May be detected)
```go
options := &mouse.ClickOptions{
    Button: mouse.LeftButton,
    Delay:  50,
    RandomDelay:  false,  // ❌ No random delay
    RestoreFocus: false,  // ❌ Don't restore focus
}
```

### Advanced Tips
1. **Vary click intervals** - Different intervals between each click
2. **Add slight offsets** - ±2 pixel random at click position
3. **Use multi-scale matching** - Adapt to window scaling
4. **Random delays** - Simulate human operation uncertainty

---

## 📁 Project Structure

```
go-game-vision/
├── pkg/                        # Core library code
│   ├── capture/               # Screen capture module
│   │   ├── capture.go         # Cross-platform interface
│   │   ├── capture_windows.go # Windows implementation
│   │   └── capture_darwin.go  # macOS implementation
│   ├── image/                 # Image processing module
│   │   └── compare.go         # Image comparison, multi-scale matching
│   ├── mouse/                 # Mouse simulation module
│   │   ├── mouse.go           # Cross-platform interface
│   │   ├── mouse_windows.go   # Windows impl (SendInput/PostMessage)
│   │   └── mouse_darwin.go    # macOS implementation
│   ├── process/               # Process management module
│   │   ├── process.go         # Cross-platform interface
│   │   ├── process_windows.go # Windows implementation
│   │   └── process_darwin.go  # macOS implementation
│   └── utils/                 # Utility module
│       ├── logger.go          # Logging system
│       └── errors.go          # Error handling
├── tests/                     # Test files
│   ├── capture_test.go        # Capture tests
│   ├── image_compare_test.go  # Image comparison tests
│   ├── mouse_test.go          # Mouse simulation tests
│   ├── process_test.go        # Process management tests
│   └── nikke_click_test.go    # Integration test example
├── go.mod                     # Go module config
├── go.sum                     # Dependency lock
├── LICENSE                    # MIT License
├── README.md                  # Chinese documentation
└── README_EN.md               # English documentation
```

---

## 🧪 Running Tests

```bash
# Run all tests (requires OpenCV)
go test -v ./...

# Run unit tests (no OpenCV dependency)
go test -v -short ./pkg/process/... ./pkg/utils/...

# Run specific tests
go test -v ./tests/ -run TestProcessManager

# Disable test cache
go test -v -count=1 ./tests/...
```

---

## ⚠️ Notes

### Windows Platform
- ✅ Some system processes require administrator privileges
- ✅ PrintWindow API can capture obscured windows
- ✅ Supports DPI scaling
- ✅ SendInput through hardware input queue, harder to detect

### macOS Platform
- ✅ Requires screen recording permission (System Preferences → Security & Privacy)
- ✅ Some system processes may not be capturable
- ✅ Uses Core Graphics API

### Performance Optimization
- 🔸 Reuse `ScreenCapture` instance for bulk screenshots
- 🔸 Image comparison performance depends on image size and algorithm
- 🔸 Recommend multi-scale matching over multiple single-scale matches
- 🔸 Suggest adding delays (50-100ms) for click operations

---

## 🤝 Contributing

Contributions, issues, and suggestions are welcome!

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Create a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) (to be added) for details

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details

---

## 🌟 Star History

If this project helps you, please give it a Star ⭐!

---

## 📮 Contact

- **Issues**: [GitHub Issues](https://github.com/lnatpunblhna/go-game-vision/issues)
- **Discussions**: [GitHub Discussions](https://github.com/lnatpunblhna/go-game-vision/discussions)

---

<div align="center">

**[⬆ Back to top](#go-game-vision)**

Made with ❤️ by Go Game Vision Contributors

</div>
