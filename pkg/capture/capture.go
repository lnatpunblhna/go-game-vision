package capture

import (
	"image"

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
	WindowTitle   string      // Window title to capture (optional)
}

// DefaultCaptureOptions default screenshot options
func DefaultCaptureOptions() *CaptureOptions {
	return &CaptureOptions{
		Format:        PNG,
		Quality:       90,
		IncludeHidden: true,
	}
}

// WindowInfo window information
type WindowInfo struct {
	Handle uintptr         // Window handle
	Title  string          // Window title
	PID    uint32          // Process ID
	Rect   image.Rectangle // Window position and size
}

// ScreenCapture screen capture interface
type ScreenCapture interface {
	// CaptureWindowByPID captures window by process ID
	CaptureWindowByPID(pid uint32, options *CaptureOptions) (image.Image, error)

	// CaptureWindowByHandle captures window by window handle
	CaptureWindowByHandle(handle uintptr, options *CaptureOptions) (image.Image, error)

	// CaptureScreen captures entire screen
	CaptureScreen(options *CaptureOptions) (image.Image, error)

	// GetWindowsByPID gets all windows by process ID
	GetWindowsByPID(pid uint32) ([]WindowInfo, error)

	// GetMainWindowByPID gets main window by process ID
	GetMainWindowByPID(pid uint32) (*WindowInfo, error)

	// SaveImage saves image to file
	SaveImage(img image.Image, filename string, format ImageFormat, quality int) error
}

// NewScreenCapture creates screen capture instance
func NewScreenCapture() ScreenCapture {
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
