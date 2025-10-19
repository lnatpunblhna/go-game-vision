//go:build windows

package mouse

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
)

// Windows mouse input constants
const (
	INPUT_MOUSE            = 0      // The event is a mouse event
	MOUSEEVENTF_LEFTDOWN   = 0x0002 // The left button is down
	MOUSEEVENTF_LEFTUP     = 0x0004 // The left button is up
	MOUSEEVENTF_RIGHTDOWN  = 0x0008 // The right button is down
	MOUSEEVENTF_RIGHTUP    = 0x0010 // The right button is up
	MOUSEEVENTF_MIDDLEDOWN = 0x0020 // The middle button is down
	MOUSEEVENTF_MIDDLEUP   = 0x0040 // The middle button is up
	MOUSEEVENTF_ABSOLUTE   = 0x8000 // Coordinates are mapped to absolute coordinates
	SM_CXSCREEN            = 0      // System metrics: screen width
	SM_CYSCREEN            = 1      // System metrics: screen height
)

// POINT defines a point with integer coordinates
type POINT struct {
	X, Y int32
}

// MOUSEINPUT contains information about a simulated mouse event
type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT contains information about a simulated input event
type INPUT struct {
	Type uint32
	_    [4]byte // padding for union alignment
	Mi   MOUSEINPUT
}

// WindowsMouseClicker implements MouseClicker for Windows
type WindowsMouseClicker struct{}

// newPlatformMouseClicker creates a Windows-specific mouse clicker
func newPlatformMouseClicker() MouseClicker {
	return &WindowsMouseClicker{}
}

// BackgroundClick performs a background mouse click at specified coordinates
func (w *WindowsMouseClicker) BackgroundClick(x, y int, options *ClickOptions) error {
	if options == nil {
		options = DefaultClickOptions()
	}

	// Validate coordinates
	if err := ValidateCoordinates(x, y); err != nil {
		return err
	}

	// Get screen dimensions for coordinate conversion
	width, height, err := w.GetScreenSize()
	if err != nil {
		return utils.WrapError(err, "failed to get screen size")
	}

	// Convert to absolute coordinates (0-65535 range)
	absX := int32((x * 65535) / width)
	absY := int32((y * 65535) / height)

	// Get current cursor position to restore later
	var currentPos POINT
	_, _, err = procGetCursorPos.Call(uintptr(unsafe.Pointer(&currentPos)))
	if err != nil {
		return err
	}

	// Perform the click
	err = w.performClick(absX, absY, options.Button)
	if err != nil {
		return utils.WrapError(err, "failed to perform click")
	}

	// Add delay if specified
	if options.Delay > 0 {
		time.Sleep(time.Duration(options.Delay) * time.Millisecond)
	}

	// Restore cursor position (for true background operation)
	_, _, err = procSetCursorPos.Call(uintptr(currentPos.X), uintptr(currentPos.Y))
	if err != nil {
		return err
	}

	utils.Info("Background click performed at (%d, %d) with %s button", x, y, options.Button.String())
	return nil
}

// performClick executes the actual mouse click using SendInput
func (w *WindowsMouseClicker) performClick(absX, absY int32, button MouseButton) error {
	var downFlag, upFlag uint32

	switch button {
	case LeftButton:
		downFlag = MOUSEEVENTF_LEFTDOWN
		upFlag = MOUSEEVENTF_LEFTUP
	case RightButton:
		downFlag = MOUSEEVENTF_RIGHTDOWN
		upFlag = MOUSEEVENTF_RIGHTUP
	case MiddleButton:
		downFlag = MOUSEEVENTF_MIDDLEDOWN
		upFlag = MOUSEEVENTF_MIDDLEUP
	default:
		return fmt.Errorf("unsupported mouse button: %v", button)
	}

	// Create input events for mouse down and up
	inputs := []INPUT{
		{
			Type: INPUT_MOUSE,
			Mi: MOUSEINPUT{
				Dx:      absX,
				Dy:      absY,
				DwFlags: downFlag | MOUSEEVENTF_ABSOLUTE,
			},
		},
		{
			Type: INPUT_MOUSE,
			Mi: MOUSEINPUT{
				Dx:      absX,
				Dy:      absY,
				DwFlags: upFlag | MOUSEEVENTF_ABSOLUTE,
			},
		},
	}

	// Send the input events
	ret, _, err := procSendInput.Call(
		uintptr(2), // number of inputs
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(INPUT{}),
	)

	if ret == 0 {
		return utils.WrapError(err, "SendInput failed")
	}

	return nil
}

// GetScreenSize returns the screen dimensions
func (w *WindowsMouseClicker) GetScreenSize() (width, height int, err error) {
	w32, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	h32, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)

	width = int(w32)
	height = int(h32)

	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid screen dimensions: %dx%d", width, height)
	}

	utils.Debug("Screen size: %dx%d", width, height)
	return width, height, nil
}

// IsValidCoordinate checks if the given coordinates are within screen bounds
func (w *WindowsMouseClicker) IsValidCoordinate(x, y int) bool {
	width, height, err := w.GetScreenSize()
	if err != nil {
		utils.Error("Failed to get screen size for coordinate validation: %v", err)
		return false
	}

	return x >= 0 && y >= 0 && x < width && y < height
}
