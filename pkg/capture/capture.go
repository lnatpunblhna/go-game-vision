package capture

import (
	"image"

	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// ImageFormat image format
type ImageFormat int

const (
	PNG ImageFormat = iota
	JPEG
	BMP
	GIF
)

// String returns format string
func (f ImageFormat) String() string {
	switch f {
	case PNG:
		return "png"
	case JPEG:
		return "jpeg"
	case BMP:
		return "bmp"
	case GIF:
		return "gif"
	default:
		return "png"
	}
}

// CaptureOptions screenshot options
type CaptureOptions struct {
	Format        ImageFormat // Image format
	Quality       int         // JPEG quality (1-100)
	IncludeHidden bool        // Whether to include hidden window content

	// WindowTitle selects among a process's windows by title substring
	// (case-insensitive). Empty means "use the process's main window".
	//
	// Set this when a process owns several windows — a launcher plus the game, or
	// a main window plus helper panels — because otherwise the choice falls to
	// whichever window the OS enumerates first, which is not stable.
	//
	// This narrows within one PID. When the process name itself is unhelpful —
	// most notably a Wine or CrossOver game, where the process is
	// `wine64-preloader` and the game's name appears only in the window title —
	// use FindWindowByTitle instead, which does not need a PID at all.
	WindowTitle string
}

// DefaultCaptureOptions default screenshot options
func DefaultCaptureOptions() *CaptureOptions {
	return &CaptureOptions{
		Format:        PNG,
		Quality:       90,
		IncludeHidden: true,
	}
}

// WindowInfo window information.
//
// Coordinate space contract — read this before doing any arithmetic with the
// fields, because Rect and captured images do NOT necessarily share a scale:
//
//   - Rect is in the platform's *click* coordinate space: the space that
//     mouse.BackgroundClick expects. On macOS that is logical points (what
//     CGEventPost consumes); on Windows it is physical pixels (the process is
//     made per-monitor DPI aware, see pkg/platform).
//   - Images returned by CaptureWindowByPID are in *physical pixels*.
//   - ScaleFactor is the ratio between the two: capturedPixels / Rect size.
//     On a Retina Mac this is 2.0; on Windows it is 1.0.
//
// So a template match found at pixel (px, py) inside a captured image maps to
// the click point Rect.Min + (px, py)/ScaleFactor. Use ScaleOrDefault rather
// than reading ScaleFactor directly, so a zero value degrades to 1.0 instead
// of dividing by zero.
type WindowInfo struct {
	Handle      uintptr         // Window handle (Windows) or Window ID (macOS)
	Title       string          // Window title
	PID         uint32          // Process ID
	Rect        image.Rectangle // Window position and size, in click coordinates
	IsHidden    bool            // Whether window is hidden/minimized
	ScaleFactor float64         // capturedPixels / Rect size (2.0 on Retina, 1.0 on Windows)
}

// ScaleOrDefault returns the window's scale factor, falling back to 1.0 when it
// is unset or nonsensical. Always prefer this over reading ScaleFactor directly:
// a WindowInfo built by hand (or by an older platform backend) leaves the field
// at 0, and dividing by that would send clicks to infinity.
func (w *WindowInfo) ScaleOrDefault() float64 {
	if w == nil || w.ScaleFactor <= 0 {
		return 1.0
	}
	return w.ScaleFactor
}

// PixelToClickPoint converts a point expressed in captured-image pixels
// (relative to the window's top-left) into absolute click coordinates.
func (w *WindowInfo) PixelToClickPoint(px, py int) image.Point {
	if w == nil {
		return image.Point{X: px, Y: py}
	}
	scale := w.ScaleOrDefault()
	return image.Point{
		X: w.Rect.Min.X + int(float64(px)/scale+0.5),
		Y: w.Rect.Min.Y + int(float64(py)/scale+0.5),
	}
}

// ScreenCapture screen capture interface
type ScreenCapture interface {
	// CaptureWindowByPID captures window by process ID
	CaptureWindowByPID(pid uint32, options *CaptureOptions) (image.Image, error)

	// SaveImage saves image to file
	SaveImage(img image.Image, filename string, format ImageFormat, quality int) error

	// GetWindowInfoByPID gets window information by process ID (optional, platform-specific)
	GetWindowInfoByPID(pid uint32) (*WindowInfo, error)
}

// NewScreenCapture creates screen capture instance.
//
// Ensures the process is DPI aware first: on Windows, GetWindowRect only
// reports true physical pixels once that opt-in has happened, and without it
// the window rectangles this package hands back would disagree with the
// captured images they are supposed to describe.
func NewScreenCapture() ScreenCapture {
	platform.EnsureDPIAware()
	return newPlatformCapture()
}

// CaptureWindowByPID convenience function: capture main window by process ID
func CaptureWindowByPID(pid uint32, options *CaptureOptions) (image.Image, error) {
	if options == nil {
		options = DefaultCaptureOptions()
	}

	capture := NewScreenCapture()
	img, err := capture.CaptureWindowByPID(pid, options)
	if err != nil {
		utils.Error("Failed to capture window PID=%d: %v", pid, err)
		return nil, utils.WrapError(err, "failed to capture window")
	}

	utils.Info("Successfully captured window PID=%d", pid)
	return img, nil
}

// CaptureAndSave convenience function: capture window and save to file
func CaptureAndSave(pid uint32, filename string, format ImageFormat, quality int) error {
	options := &CaptureOptions{
		Format:        format,
		Quality:       quality,
		IncludeHidden: true,
	}

	capture := NewScreenCapture()
	img, err := capture.CaptureWindowByPID(pid, options)
	if err != nil {
		return utils.WrapError(err, "failed to capture window")
	}

	err = capture.SaveImage(img, filename, format, quality)
	if err != nil {
		return utils.WrapError(err, "failed to save image")
	}

	utils.Info("Successfully captured and saved window PID=%d to file: %s", pid, filename)
	return nil
}

// GetWindowInfoByPID convenience function: get window information by process ID
func GetWindowInfoByPID(pid uint32) (*WindowInfo, error) {
	capture := NewScreenCapture()
	info, err := capture.GetWindowInfoByPID(pid)
	if err != nil {
		utils.Error("Failed to get window info for PID=%d: %v", pid, err)
		return nil, utils.WrapError(err, "failed to get window info")
	}

	utils.Info("Successfully got window info for PID=%d", pid)
	return info, nil
}
