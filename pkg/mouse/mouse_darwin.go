//go:build darwin

package mouse

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <unistd.h>

// Perform background mouse click
int performBackgroundClick(double x, double y, int button, int delay) {
    CGEventType downEventType, upEventType;
    CGMouseButton mouseButton;

    // Determine event types and button based on input
    switch (button) {
        case 0: // Left button
            downEventType = kCGEventLeftMouseDown;
            upEventType = kCGEventLeftMouseUp;
            mouseButton = kCGMouseButtonLeft;
            break;
        case 1: // Right button
            downEventType = kCGEventRightMouseDown;
            upEventType = kCGEventRightMouseUp;
            mouseButton = kCGMouseButtonRight;
            break;
        case 2: // Middle button
            downEventType = kCGEventOtherMouseDown;
            upEventType = kCGEventOtherMouseUp;
            mouseButton = kCGMouseButtonCenter;
            break;
        default:
            return -1; // Invalid button
    }

    CGPoint clickPoint = CGPointMake(x, y);

    // Create mouse down event
    CGEventRef mouseDownEvent = CGEventCreateMouseEvent(NULL, downEventType, clickPoint, mouseButton);
    if (mouseDownEvent == NULL) {
        return -2; // Failed to create down event
    }

    // Create mouse up event
    CGEventRef mouseUpEvent = CGEventCreateMouseEvent(NULL, upEventType, clickPoint, mouseButton);
    if (mouseUpEvent == NULL) {
        CFRelease(mouseDownEvent);
        return -3; // Failed to create up event
    }

    // Post the events to the system
    CGEventPost(kCGHIDEventTap, mouseDownEvent);

    // Add delay between down and up events
    if (delay > 0) {
        usleep(delay * 1000); // Convert milliseconds to microseconds
    }

    CGEventPost(kCGHIDEventTap, mouseUpEvent);

    // Clean up
    CFRelease(mouseDownEvent);
    CFRelease(mouseUpEvent);

    return 0; // Success
}

// Get screen dimensions
void getScreenSize(int* width, int* height) {
    CGDirectDisplayID displayID = CGMainDisplayID();
    *width = (int)CGDisplayPixelsWide(displayID);
    *height = (int)CGDisplayPixelsHigh(displayID);
}

// Check if coordinates are valid
int isValidCoordinate(double x, double y) {
    int width, height;
    getScreenSize(&width, &height);
    return (x >= 0 && y >= 0 && x < width && y < height) ? 1 : 0;
}

// Perform a drag: press at the start, emit motion along the path, release at the end.
//
// The intermediate kCGEventLeftMouseDragged events are the point of this function.
// A press at A followed by a release at B is usually interpreted as a click,
// because applications recognise dragging by observing motion while the button is
// held — and a drag with no motion has none to observe.
int performDrag(double startX, double startY,
                double* pathX, double* pathY, int pathLen,
                int button, int holdMS, int stepDelayMS) {
    CGEventType downType, upType, dragType;
    CGMouseButton mouseButton;

    switch (button) {
        case 0:
            downType = kCGEventLeftMouseDown;
            upType = kCGEventLeftMouseUp;
            dragType = kCGEventLeftMouseDragged;
            mouseButton = kCGMouseButtonLeft;
            break;
        case 1:
            downType = kCGEventRightMouseDown;
            upType = kCGEventRightMouseUp;
            dragType = kCGEventRightMouseDragged;
            mouseButton = kCGMouseButtonRight;
            break;
        case 2:
            downType = kCGEventOtherMouseDown;
            upType = kCGEventOtherMouseUp;
            dragType = kCGEventOtherMouseDragged;
            mouseButton = kCGMouseButtonCenter;
            break;
        default:
            return -1;
    }

    CGEventRef down = CGEventCreateMouseEvent(NULL, downType,
        CGPointMake(startX, startY), mouseButton);
    if (down == NULL) return -2;
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);

    if (holdMS > 0) usleep(holdMS * 1000);

    for (int i = 0; i < pathLen; i++) {
        CGEventRef move = CGEventCreateMouseEvent(NULL, dragType,
            CGPointMake(pathX[i], pathY[i]), mouseButton);
        if (move == NULL) continue;
        CGEventPost(kCGHIDEventTap, move);
        CFRelease(move);
        if (stepDelayMS > 0) usleep(stepDelayMS * 1000);
    }

    if (holdMS > 0) usleep(holdMS * 1000);

    double endX = pathLen > 0 ? pathX[pathLen - 1] : startX;
    double endY = pathLen > 0 ? pathY[pathLen - 1] : startY;
    CGEventRef up = CGEventCreateMouseEvent(NULL, upType,
        CGPointMake(endX, endY), mouseButton);
    if (up == NULL) return -3;
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);

    return 0;
}

// Scroll by whole wheel notches at a point.
//
// The cursor is moved first: scroll events go to whatever is under the pointer,
// so scrolling without positioning would hit whichever view happened to be there.
// One notch per event rather than one event of N units, because large single
// deltas are frequently coalesced into less movement than requested.
int performScroll(double x, double y, int amount, int horizontal, int stepDelayMS) {
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved,
        CGPointMake(x, y), kCGMouseButtonLeft);
    if (move != NULL) {
        CGEventPost(kCGHIDEventTap, move);
        CFRelease(move);
    }

    int steps = amount < 0 ? -amount : amount;
    int direction = amount < 0 ? -1 : 1;

    for (int i = 0; i < steps; i++) {
        CGEventRef scroll;
        if (horizontal) {
            // Axis 2 is horizontal; it needs two axes to be declared.
            scroll = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, 0, direction);
        } else {
            scroll = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, direction);
        }
        if (scroll == NULL) return -1;
        CGEventPost(kCGHIDEventTap, scroll);
        CFRelease(scroll);
        if (stepDelayMS > 0) usleep(stepDelayMS * 1000);
    }
    return 0;
}

// Read the current cursor position in logical points.
//
// Needed by the emergency-stop watchdog, which polls for the cursor reaching a
// screen corner. Creating a null event and reading its location is the standard
// way to query this and, unlike an event tap, needs no Accessibility permission —
// which matters for a safety mechanism that must work without extra setup.
int getCursorPosition(double* x, double* y) {
    CGEventRef event = CGEventCreate(NULL);
    if (event == NULL) {
        return -1;
    }
    CGPoint point = CGEventGetLocation(event);
    CFRelease(event);
    *x = point.x;
    *y = point.y;
    return 0;
}
*/
import "C"
import (
	"fmt"
	"image"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// DarwinMouseClicker implements MouseClicker for macOS
type DarwinMouseClicker struct{}

// newPlatformMouseClicker creates a macOS-specific mouse clicker
func newPlatformMouseClicker() MouseClicker {
	return &DarwinMouseClicker{}
}

// BackgroundClick performs a background mouse click at specified coordinates
func (d *DarwinMouseClicker) BackgroundClick(x, y int, options *ClickOptions) error {
	if options == nil {
		options = DefaultClickOptions()
	}

	// Validate coordinates
	if err := ValidateCoordinates(x, y); err != nil {
		return err
	}

	// Convert MouseButton to C int
	var buttonCode int
	switch options.Button {
	case LeftButton:
		buttonCode = 0
	case RightButton:
		buttonCode = 1
	case MiddleButton:
		buttonCode = 2
	default:
		return fmt.Errorf("unsupported mouse button: %v", options.Button)
	}

	// Perform the click using C function
	result := C.performBackgroundClick(C.double(x), C.double(y), C.int(buttonCode), C.int(options.Delay))

	switch result {
	case 0:
		utils.Info("Background click performed at (%d, %d) with %s button", x, y, options.Button.String())
		return nil
	case -1:
		return fmt.Errorf("invalid mouse button: %v", options.Button)
	case -2:
		return fmt.Errorf("failed to create mouse down event")
	case -3:
		return fmt.Errorf("failed to create mouse up event")
	default:
		return fmt.Errorf("unknown error occurred during click: %d", result)
	}
}

// GetScreenSize returns the screen dimensions
func (d *DarwinMouseClicker) GetScreenSize() (width, height int, err error) {
	var w, h C.int
	C.getScreenSize(&w, &h)

	width = int(w)
	height = int(h)

	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid screen dimensions: %dx%d", width, height)
	}

	utils.Debug("Screen size: %dx%d", width, height)
	return width, height, nil
}

// IsValidCoordinate checks if the given coordinates are within screen bounds
func (d *DarwinMouseClicker) IsValidCoordinate(x, y int) bool {
	result := C.isValidCoordinate(C.double(x), C.double(y))
	return result == 1
}

// PostMessageClick is not supported on macOS.
//
// The "post a message to a window without activating it" model is a Win32
// concept (WM_LBUTTONDOWN/PostMessageW). macOS has no equivalent public API to
// deliver synthetic mouse events to a specific window without the window/app
// becoming active, so this method is intentionally unimplemented on darwin.
//
// Callers that need a click on macOS should use BackgroundClick with screen
// coordinates instead.
func (d *DarwinMouseClicker) PostMessageClick(hwnd uintptr, x, y int, options *ClickOptions) error {
	return utils.WrapError(utils.ErrPlatformNotSupported,
		"PostMessageClick is only available on Windows; use BackgroundClick on macOS")
}

// getPlatformCursorPosition reads the current cursor location in logical points,
// the same coordinate space BackgroundClick accepts.
func getPlatformCursorPosition() (int, int, error) {
	var x, y C.double
	if C.getCursorPosition(&x, &y) != 0 {
		return 0, 0, utils.WrapError(utils.ErrPlatformNotSupported, "failed to read cursor position")
	}
	return int(x), int(y), nil
}

// dragPlatform performs a drag using CoreGraphics events.
//
// The intermediate kCGEventLeftMouseDragged events are what make this a drag
// rather than two unrelated clicks: applications detect dragging by observing
// motion while a button is held.
func dragPlatform(from, to image.Point, opts *DragOptions) error {
	buttonCode, err := darwinButtonCode(opts.button())
	if err != nil {
		return err
	}

	steps := opts.steps()
	xs := make([]C.double, 0, steps+1)
	ys := make([]C.double, 0, steps+1)
	for i := 1; i <= steps; i++ {
		p := interpolate(from, to, float64(i)/float64(steps))
		xs = append(xs, C.double(p.X))
		ys = append(ys, C.double(p.Y))
	}
	if len(xs) == 0 {
		xs = append(xs, C.double(to.X))
		ys = append(ys, C.double(to.Y))
	}

	ret := C.performDrag(
		C.double(from.X), C.double(from.Y),
		&xs[0], &ys[0], C.int(len(xs)),
		C.int(buttonCode),
		C.int(opts.holdMS()), C.int(opts.stepDelayMS()),
	)
	if ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("drag failed with code %d", int(ret)))
	}
	return nil
}

// scrollPlatform scrolls using a CoreGraphics scroll wheel event.
func scrollPlatform(x, y int, amount int, opts *ScrollOptions) error {
	horizontal := 0
	if opts.horizontal() {
		horizontal = 1
	}
	ret := C.performScroll(
		C.double(x), C.double(y),
		C.int(amount), C.int(horizontal), C.int(opts.stepDelayMS()),
	)
	if ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("scroll failed with code %d", int(ret)))
	}
	return nil
}

// darwinButtonCode maps a MouseButton to the integer the C helpers expect.
func darwinButtonCode(button MouseButton) (int, error) {
	switch button {
	case LeftButton:
		return 0, nil
	case RightButton:
		return 1, nil
	case MiddleButton:
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported mouse button: %v", button)
	}
}
