//go:build windows

package mouse

import (
	"fmt"
	"image"
	"math/rand"
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

// DPI awareness used to be opted into by an init() here. It now lives in
// pkg/platform, which pkg/capture depends on too — a program that imported only
// pkg/capture used to miss this call entirely and read back DPI-virtualised
// window rectangles. pkg/platform also upgrades the request from the legacy
// system-DPI-aware call to per-monitor-v2, which is the only mode that is
// correct on mixed-scale multi-monitor setups.

// Windows mouse input constants
const (
	INPUT_MOUSE             = 0      // The event is a mouse event
	MOUSEEVENTF_MOVE        = 0x0001 // Movement occurred
	MOUSEEVENTF_LEFTDOWN    = 0x0002 // The left button is down
	MOUSEEVENTF_LEFTUP      = 0x0004 // The left button is up
	MOUSEEVENTF_RIGHTDOWN   = 0x0008 // The right button is down
	MOUSEEVENTF_RIGHTUP     = 0x0010 // The right button is up
	MOUSEEVENTF_MIDDLEDOWN  = 0x0020 // The middle button is down
	MOUSEEVENTF_MIDDLEUP    = 0x0040 // The middle button is up
	MOUSEEVENTF_ABSOLUTE    = 0x8000 // Coordinates are mapped to absolute coordinates
	MOUSEEVENTF_VIRTUALDESK = 0x4000 // Map absolute coords to the entire virtual desktop (multi-monitor)
	SM_CXSCREEN             = 0      // System metrics: primary screen width
	SM_CYSCREEN             = 1      // System metrics: primary screen height
	SM_XVIRTUALSCREEN       = 76     // Left of the virtual screen (may be negative)
	SM_YVIRTUALSCREEN       = 77     // Top of the virtual screen (may be negative)
	SM_CXVIRTUALSCREEN      = 78     // Width of the virtual screen (all monitors)
	SM_CYVIRTUALSCREEN      = 79     // Height of the virtual screen (all monitors)
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

	// Convert to absolute coordinates (0-65535) relative to the VIRTUAL desktop
	// so that clicks on secondary monitors (including negative coordinates) map
	// correctly. Uses MOUSEEVENTF_VIRTUALDESK in performClick.
	vx, vy, vw, vh, err := w.getVirtualScreenRect()
	if err != nil {
		return utils.WrapError(err, "failed to get virtual screen size")
	}
	absX := int32(((x - vx) * 65535) / (vw - 1))
	absY := int32(((y - vy) * 65535) / (vh - 1))

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
		randomMs := 5 + rand.Intn(10) // 5-14ms random
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
		randomMs := 3 + rand.Intn(7) // 3-9ms random
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
				DwFlags: downFlag | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK | MOUSEEVENTF_MOVE,
			},
		},
		{
			Type: INPUT_MOUSE,
			Mi: MOUSEINPUT{
				Dx:      absX,
				Dy:      absY,
				DwFlags: upFlag | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK | MOUSEEVENTF_MOVE,
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

// getVirtualScreenRect returns the bounding rectangle (origin x/y, width, height)
// of the virtual screen spanning all monitors. The origin may be negative when
// a secondary monitor sits to the left of / above the primary one.
func (w *WindowsMouseClicker) getVirtualScreenRect() (x, y, width, height int, err error) {
	vx, _, _ := procGetSystemMetrics.Call(SM_XVIRTUALSCREEN)
	vy, _, _ := procGetSystemMetrics.Call(SM_YVIRTUALSCREEN)
	vw, _, _ := procGetSystemMetrics.Call(SM_CXVIRTUALSCREEN)
	vh, _, _ := procGetSystemMetrics.Call(SM_CYVIRTUALSCREEN)

	// GetSystemMetrics returns signed ints; SM_X/YVIRTUALSCREEN can be negative.
	x = int(int32(vx))
	y = int(int32(vy))
	width = int(int32(vw))
	height = int(int32(vh))

	if width <= 1 || height <= 1 {
		// Fall back to the primary display if virtual metrics are unavailable.
		pw, ph, perr := w.GetScreenSize()
		if perr != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid virtual screen dimensions: %dx%d", width, height)
		}
		return 0, 0, pw, ph, nil
	}
	return x, y, width, height, nil
}

// IsValidCoordinate checks if the given coordinates are within the virtual
// screen bounds (all monitors), so clicks on secondary displays are accepted.
func (w *WindowsMouseClicker) IsValidCoordinate(x, y int) bool {
	vx, vy, vw, vh, err := w.getVirtualScreenRect()
	if err != nil {
		utils.Error("Failed to get virtual screen size for coordinate validation: %v", err)
		return false
	}

	return x >= vx && y >= vy && x < vx+vw && y < vy+vh
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

// getPlatformCursorPosition reads the current cursor location in physical pixels,
// the same coordinate space BackgroundClick accepts once the process is
// per-monitor DPI aware (see pkg/platform).
func getPlatformCursorPosition() (int, int, error) {
	var pos POINT
	ret, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pos)))
	if ret == 0 {
		return 0, 0, utils.WrapError(err, "GetCursorPos failed")
	}
	return int(pos.X), int(pos.Y), nil
}

// Mouse wheel constants.
const (
	MOUSEEVENTF_WHEEL  = 0x0800 // Vertical wheel movement
	MOUSEEVENTF_HWHEEL = 0x1000 // Horizontal wheel movement
	WHEEL_DELTA        = 120    // One notch of wheel rotation
)

// sendMouseEvent posts a single mouse event via SendInput.
//
// Absolute coordinates are expressed against the virtual desktop
// (MOUSEEVENTF_VIRTUALDESK) so that points on secondary monitors — which can have
// negative coordinates — map correctly rather than being clamped to the primary
// display.
func sendMouseEvent(x, y int, flags uint32, mouseData uint32) error {
	vx, vy, vw, vh, err := (&WindowsMouseClicker{}).getVirtualScreenRect()
	if err != nil {
		return utils.WrapError(err, "failed to get virtual screen size")
	}

	input := INPUT{
		Type: INPUT_MOUSE,
		Mi: MOUSEINPUT{
			Dx:        int32(((x - vx) * 65535) / (vw - 1)),
			Dy:        int32(((y - vy) * 65535) / (vh - 1)),
			MouseData: mouseData,
			DwFlags:   flags | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK,
		},
	}

	ret, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if ret != 1 {
		return utils.WrapError(callErr, "SendInput failed")
	}
	return nil
}

// dragPlatform performs a drag using SendInput.
func dragPlatform(from, to image.Point, opts *DragOptions) error {
	var downFlag, upFlag uint32
	switch opts.button() {
	case LeftButton:
		downFlag, upFlag = MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP
	case RightButton:
		downFlag, upFlag = MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP
	case MiddleButton:
		downFlag, upFlag = MOUSEEVENTF_MIDDLEDOWN, MOUSEEVENTF_MIDDLEUP
	default:
		return fmt.Errorf("unsupported mouse button: %v", opts.button())
	}

	// Move to the start before pressing: pressing at a position the cursor has not
	// reached yet is how drags end up starting from wherever the cursor last was.
	if err := sendMouseEvent(from.X, from.Y, MOUSEEVENTF_MOVE, 0); err != nil {
		return err
	}
	if err := sendMouseEvent(from.X, from.Y, downFlag, 0); err != nil {
		return err
	}

	// Release the button even if the motion fails, so a failure cannot leave the
	// button stuck down system-wide.
	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		return sendMouseEvent(to.X, to.Y, upFlag, 0)
	}
	defer func() {
		if err := release(); err != nil {
			utils.Warn("failed to release the drag button: %v", err)
		}
	}()

	time.Sleep(time.Duration(opts.holdMS()) * time.Millisecond)

	steps := opts.steps()
	for i := 1; i <= steps; i++ {
		p := interpolate(from, to, float64(i)/float64(steps))
		if err := sendMouseEvent(p.X, p.Y, MOUSEEVENTF_MOVE, 0); err != nil {
			return err
		}
		time.Sleep(time.Duration(opts.stepDelayMS()) * time.Millisecond)
	}

	time.Sleep(time.Duration(opts.holdMS()) * time.Millisecond)
	return release()
}

// scrollPlatform scrolls using SendInput wheel events.
func scrollPlatform(x, y int, amount int, opts *ScrollOptions) error {
	flag := uint32(MOUSEEVENTF_WHEEL)
	if opts.horizontal() {
		flag = MOUSEEVENTF_HWHEEL
	}

	// Position the cursor first: wheel events are delivered to whatever is under
	// the pointer.
	if err := sendMouseEvent(x, y, MOUSEEVENTF_MOVE, 0); err != nil {
		return err
	}

	steps := amount
	direction := 1
	if steps < 0 {
		steps, direction = -steps, -1
	}

	// One notch per event rather than one event carrying N notches: large single
	// deltas are often coalesced into less scrolling than asked for.
	delta := int32(direction * WHEEL_DELTA)
	for i := 0; i < steps; i++ {
		if err := sendMouseEvent(x, y, flag, uint32(delta)); err != nil {
			return err
		}
		time.Sleep(time.Duration(opts.stepDelayMS()) * time.Millisecond)
	}
	return nil
}
