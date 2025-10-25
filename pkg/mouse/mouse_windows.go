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
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSendInput           = user32.NewProc("SendInput")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetCursorPos        = user32.NewProc("SetCursorPos")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procPostMessage         = user32.NewProc("PostMessageW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procWindowFromPoint     = user32.NewProc("WindowFromPoint")
	procScreenToClient      = user32.NewProc("ScreenToClient")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// Windows mouse input constants
const (
	INPUT_MOUSE            = 0      // The event is a mouse event
	MOUSEEVENTF_MOVE       = 0x0001 // Movement occurred
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

// Windows message constants for PostMessage/SendMessage
const (
	WM_MOUSEMOVE   = 0x0200 // Mouse move message
	WM_LBUTTONDOWN = 0x0201 // Left button down message
	WM_LBUTTONUP   = 0x0202 // Left button up message
	WM_RBUTTONDOWN = 0x0204 // Right button down message
	WM_RBUTTONUP   = 0x0205 // Right button up message
	WM_MBUTTONDOWN = 0x0207 // Middle button down message
	WM_MBUTTONUP   = 0x0208 // Middle button up message
)

// Mouse button state flags for wParam
const (
	MK_LBUTTON = 0x0001 // Left button is down
	MK_RBUTTON = 0x0002 // Right button is down
	MK_MBUTTON = 0x0010 // Middle button is down
)

// POINT defines a point with integer coordinates
type POINT struct {
	X, Y int32
}

// RECT defines a rectangle with integer coordinates
type RECT struct {
	Left, Top, Right, Bottom int32
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
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&currentPos)))
	if ret == 0 {
		return utils.WrapError(fmt.Errorf("GetCursorPos failed"), "failed to get cursor position")
	}

	// Get current foreground window to restore focus if needed
	var originalForeground uintptr
	if options.RestoreFocus {
		originalForeground, _, _ = procGetForegroundWindow.Call()
	}

	// Add random pre-delay if requested (simulates human hesitation)
	if options.RandomDelay {
		randomMs := 5 + (time.Now().UnixNano() % 10) // 5-14ms random
		time.Sleep(time.Duration(randomMs) * time.Millisecond)
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

	// Add random post-delay if requested
	if options.RandomDelay {
		randomMs := 3 + (time.Now().UnixNano() % 7) // 3-9ms random
		time.Sleep(time.Duration(randomMs) * time.Millisecond)
	}

	// Restore cursor position (for true background operation)
	ret, _, _ = procSetCursorPos.Call(uintptr(currentPos.X), uintptr(currentPos.Y))
	if ret == 0 {
		return utils.WrapError(fmt.Errorf("SetCursorPos failed"), "failed to restore cursor position")
	}

	// Restore original foreground window if requested
	if options.RestoreFocus && originalForeground != 0 {
		procSetForegroundWindow.Call(originalForeground)
		utils.Debug("Restored focus to original window (hwnd: 0x%X)", originalForeground)
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
				DwFlags: downFlag | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_MOVE,
			},
		},
		{
			Type: INPUT_MOUSE,
			Mi: MOUSEINPUT{
				Dx:      absX,
				Dy:      absY,
				DwFlags: upFlag | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_MOVE,
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

// PostMessageClick performs a true background click using SendMessage API
// This method does NOT activate the target window
func (w *WindowsMouseClicker) PostMessageClick(hwnd uintptr, x, y int, options *ClickOptions) error {
	if options == nil {
		options = DefaultClickOptions()
	}

	if hwnd == 0 {
		return fmt.Errorf("invalid window handle")
	}

	// Get message codes and flags for the button
	var downMsg, upMsg uint32
	var wParamDown uintptr
	switch options.Button {
	case LeftButton:
		downMsg = WM_LBUTTONDOWN
		upMsg = WM_LBUTTONUP
		wParamDown = MK_LBUTTON
	case RightButton:
		downMsg = WM_RBUTTONDOWN
		upMsg = WM_RBUTTONUP
		wParamDown = MK_RBUTTON
	case MiddleButton:
		downMsg = WM_MBUTTONDOWN
		upMsg = WM_MBUTTONUP
		wParamDown = MK_MBUTTON
	default:
		return fmt.Errorf("unsupported mouse button: %v", options.Button)
	}

	// Encode coordinates into lParam (low word = x, high word = y)
	lParam := uintptr(x&0xFFFF | (y&0xFFFF)<<16)

	// Step 1: Send WM_MOUSEMOVE to position the cursor
	utils.Debug("Sending WM_MOUSEMOVE to (%d, %d)", x, y)
	procSendMessage.Call(hwnd, WM_MOUSEMOVE, 0, lParam)
	time.Sleep(10 * time.Millisecond)

	// Step 2: Send mouse button down message with button state flag
	utils.Debug("Sending button down message (msg=0x%X, wParam=0x%X, lParam=0x%X)", downMsg, wParamDown, lParam)
	procSendMessage.Call(hwnd, uintptr(downMsg), wParamDown, lParam)

	// Step 3: Add delay between down and up
	if options.Delay > 0 {
		time.Sleep(time.Duration(options.Delay) * time.Millisecond)
	} else {
		time.Sleep(10 * time.Millisecond)
	}

	// Step 4: Send mouse button up message (no button flag in wParam when releasing)
	utils.Debug("Sending button up message (msg=0x%X, wParam=0x%X, lParam=0x%X)", upMsg, 0, lParam)
	procSendMessage.Call(hwnd, uintptr(upMsg), 0, lParam)

	utils.Info("SendMessage click performed at window-relative (%d, %d) with %s button (hwnd: 0x%X)",
		x, y, options.Button.String(), hwnd)
	return nil
}

// PostMessageClickAtScreenCoords performs a click at screen coordinates
// It automatically finds the child window at that position and converts coordinates
func (w *WindowsMouseClicker) PostMessageClickAtScreenCoords(parentHwnd uintptr, screenX, screenY int, options *ClickOptions) error {
	if options == nil {
		options = DefaultClickOptions()
	}

	// Convert screen coordinates to POINT
	pt := POINT{X: int32(screenX), Y: int32(screenY)}

	// Find the actual window at this screen position (could be a child window)
	targetHwnd, _, _ := procWindowFromPoint.Call(
		uintptr(pt.X),
		uintptr(pt.Y),
	)

	if targetHwnd == 0 {
		utils.Warn("WindowFromPoint returned NULL, using parent hwnd")
		targetHwnd = parentHwnd
	}

	utils.Debug("Screen coords (%d, %d) -> target window hwnd=0x%X (parent=0x%X)",
		screenX, screenY, targetHwnd, parentHwnd)

	// Convert screen coordinates to client coordinates of the target window
	clientPt := POINT{X: int32(screenX), Y: int32(screenY)}
	procScreenToClient.Call(targetHwnd, uintptr(unsafe.Pointer(&clientPt)))

	utils.Debug("Client coords: (%d, %d)", clientPt.X, clientPt.Y)

	// Perform click using client coordinates
	return w.postMessageClickInternal(targetHwnd, int(clientPt.X), int(clientPt.Y), options)
}

// postMessageClickInternal is the internal implementation
func (w *WindowsMouseClicker) postMessageClickInternal(hwnd uintptr, x, y int, options *ClickOptions) error {
	// Get message codes and flags for the button
	var downMsg, upMsg uint32
	var wParamDown uintptr
	switch options.Button {
	case LeftButton:
		downMsg = WM_LBUTTONDOWN
		upMsg = WM_LBUTTONUP
		wParamDown = MK_LBUTTON
	case RightButton:
		downMsg = WM_RBUTTONDOWN
		upMsg = WM_RBUTTONUP
		wParamDown = MK_RBUTTON
	case MiddleButton:
		downMsg = WM_MBUTTONDOWN
		upMsg = WM_MBUTTONUP
		wParamDown = MK_MBUTTON
	default:
		return fmt.Errorf("unsupported mouse button: %v", options.Button)
	}

	// Encode coordinates into lParam
	lParam := uintptr(x&0xFFFF | (y&0xFFFF)<<16)

	// Send WM_MOUSEMOVE
	procSendMessage.Call(hwnd, WM_MOUSEMOVE, 0, lParam)
	time.Sleep(10 * time.Millisecond)

	// Send button down
	procSendMessage.Call(hwnd, uintptr(downMsg), wParamDown, lParam)

	// Delay
	if options.Delay > 0 {
		time.Sleep(time.Duration(options.Delay) * time.Millisecond)
	} else {
		time.Sleep(10 * time.Millisecond)
	}

	// Send button up
	procSendMessage.Call(hwnd, uintptr(upMsg), 0, lParam)

	utils.Info("Click sent to hwnd=0x%X at client coords (%d, %d)", hwnd, x, y)
	return nil
}
