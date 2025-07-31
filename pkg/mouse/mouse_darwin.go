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
*/
import "C"
import (
	"fmt"

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
