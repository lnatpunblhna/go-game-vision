//go:build darwin

package keyboard

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <unistd.h>

// Post a key event with the given modifier flags.
//
// Modifier flags travel on the event rather than as separate key events: CGEvent
// carries a flags field, and setting it is both simpler and more reliable than
// synthesising down/up pairs for Shift and friends, which can leave the system in
// a stuck-modifier state if anything goes wrong in between.
int postKeyEvent(unsigned short keyCode, int down, unsigned long long flags) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, down ? true : false);
    if (event == NULL) {
        return -1;
    }
    if (flags != 0) {
        CGEventSetFlags(event, (CGEventFlags)flags);
    }
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

// Post a single unicode character, bypassing keycode mapping entirely.
//
// Used by TypeString: mapping arbitrary text to physical keycodes would require
// knowing the user's active layout, whereas attaching a UTF-16 payload to the
// event lets the system deliver the character directly. keyCode 0 is fine here
// because the payload, not the code, determines what arrives.
int postUnicodeChar(unsigned short* utf16, int length) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (down == NULL) {
        return -1;
    }
    CGEventKeyboardSetUnicodeString(down, length, utf16);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);

    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (up == NULL) {
        return -2;
    }
    CGEventKeyboardSetUnicodeString(up, length, utf16);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);
    return 0;
}
*/
import "C"
import (
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// darwinKeyCodes maps Key to macOS virtual key codes.
//
// These are positional codes from Carbon's Events.h (kVK_ANSI_*), named after
// where the key sits on a US layout rather than what it types. The ordering looks
// arbitrary because it reflects the original Apple Extended Keyboard's hardware
// scan order, not the alphabet — so the table has to be written out rather than
// computed.
var darwinKeyCodes = map[Key]C.ushort{
	KeyA: 0x00, KeyS: 0x01, KeyD: 0x02, KeyF: 0x03, KeyH: 0x04, KeyG: 0x05,
	KeyZ: 0x06, KeyX: 0x07, KeyC: 0x08, KeyV: 0x09, KeyB: 0x0B, KeyQ: 0x0C,
	KeyW: 0x0D, KeyE: 0x0E, KeyR: 0x0F, KeyY: 0x10, KeyT: 0x11,
	Key1: 0x12, Key2: 0x13, Key3: 0x14, Key4: 0x15, Key6: 0x16, Key5: 0x17,
	KeyEqual: 0x18, Key9: 0x19, Key7: 0x1A, KeyMinus: 0x1B, Key8: 0x1C, Key0: 0x1D,
	KeyRightBracket: 0x1E, KeyO: 0x1F, KeyU: 0x20, KeyLeftBracket: 0x21,
	KeyI: 0x22, KeyP: 0x23,
	KeyReturn: 0x24, KeyL: 0x25, KeyJ: 0x26, KeyQuote: 0x27, KeyK: 0x28,
	KeySemicolon: 0x29, KeyBackslash: 0x2A, KeyComma: 0x2B, KeySlash: 0x2C,
	KeyN: 0x2D, KeyM: 0x2E, KeyPeriod: 0x2F,
	KeyTab: 0x30, KeySpace: 0x31, KeyGrave: 0x32, KeyBackspace: 0x33,
	KeyEscape:  0x35,
	KeyMeta:    0x37, // Command
	KeyShift:   0x38,
	KeyAlt:     0x3A, // Option
	KeyControl: 0x3B,

	KeyF5: 0x60, KeyF6: 0x61, KeyF7: 0x62, KeyF3: 0x63, KeyF8: 0x64, KeyF9: 0x65,
	KeyF11: 0x67, KeyF10: 0x6D, KeyF12: 0x6F,
	KeyHome: 0x73, KeyPageUp: 0x74, KeyDelete: 0x75, KeyF4: 0x76,
	KeyEnd: 0x77, KeyF2: 0x78, KeyPageDown: 0x79, KeyF1: 0x7A,
	KeyLeft: 0x7B, KeyRight: 0x7C, KeyDown: 0x7D, KeyUp: 0x7E,
}

// macOS CGEventFlags values for modifier keys.
const (
	cgFlagShift   = 0x00020000 // kCGEventFlagMaskShift
	cgFlagControl = 0x00040000 // kCGEventFlagMaskControl
	cgFlagAlt     = 0x00080000 // kCGEventFlagMaskAlternate
	cgFlagMeta    = 0x00100000 // kCGEventFlagMaskCommand
)

// DarwinKeyboard implements Keyboard on macOS.
type DarwinKeyboard struct{}

func newPlatformKeyboard() Keyboard {
	return &DarwinKeyboard{}
}

// keyCode resolves a Key to a macOS virtual key code.
func keyCode(key Key) (C.ushort, error) {
	code, ok := darwinKeyCodes[key]
	if !ok {
		return 0, utils.WrapError(utils.ErrInvalidParameter,
			fmt.Sprintf("key %s has no macOS key code mapping", key))
	}
	return code, nil
}

// cgFlags converts a Modifier mask to CGEventFlags.
func cgFlags(mod Modifier) C.ulonglong {
	var flags C.ulonglong
	if mod.Has(ModShift) {
		flags |= cgFlagShift
	}
	if mod.Has(ModControl) {
		flags |= cgFlagControl
	}
	if mod.Has(ModAlt) {
		flags |= cgFlagAlt
	}
	if mod.Has(ModMeta) {
		flags |= cgFlagMeta
	}
	return flags
}

// KeyDown presses a key and leaves it held.
func (d *DarwinKeyboard) KeyDown(key Key) error {
	code, err := keyCode(key)
	if err != nil {
		return err
	}
	if ret := C.postKeyEvent(code, 1, 0); ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("failed to post key-down for %s (code %d)", key, int(ret)))
	}
	utils.Debug("key down: %s", key)
	return nil
}

// KeyUp releases a key.
func (d *DarwinKeyboard) KeyUp(key Key) error {
	code, err := keyCode(key)
	if err != nil {
		return err
	}
	if ret := C.postKeyEvent(code, 0, 0); ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("failed to post key-up for %s (code %d)", key, int(ret)))
	}
	utils.Debug("key up: %s", key)
	return nil
}

// Press presses and releases a key with modifiers applied.
func (d *DarwinKeyboard) Press(key Key, opts *PressOptions) error {
	code, err := keyCode(key)
	if err != nil {
		return err
	}
	flags := cgFlags(opts.modifiers())

	if ret := C.postKeyEvent(code, 1, flags); ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("failed to post key-down for %s (code %d)", key, int(ret)))
	}

	time.Sleep(time.Duration(opts.holdMS()) * time.Millisecond)

	if ret := C.postKeyEvent(code, 0, flags); ret != 0 {
		return utils.WrapError(utils.ErrPlatformNotSupported,
			fmt.Sprintf("failed to post key-up for %s (code %d)", key, int(ret)))
	}

	utils.Debug("pressed %s with modifiers %s", key, opts.modifiers())
	return nil
}

// TypeString types text via unicode events, independent of keyboard layout.
func (d *DarwinKeyboard) TypeString(text string) error {
	for _, r := range text {
		// CGEventKeyboardSetUnicodeString takes UTF-16, so anything outside the
		// BMP arrives as a surrogate pair rather than one unit.
		units := utf16.Encode([]rune{r})
		if len(units) == 0 {
			continue
		}
		ret := C.postUnicodeChar((*C.ushort)(unsafe.Pointer(&units[0])), C.int(len(units)))
		if ret != 0 {
			return utils.WrapError(utils.ErrPlatformNotSupported,
				fmt.Sprintf("failed to type %q (code %d)", r, int(ret)))
		}
		// A brief gap between characters; without it fast typing can arrive
		// out of order or be dropped by the receiving app.
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// PostKeyToWindow is not supported on macOS.
//
// Delivering a key to a specific window without activating it is a Win32 concept
// (PostMessageW with WM_KEYDOWN). macOS has no public equivalent, for the same
// reason MouseClicker.PostMessageClick is Windows-only.
func (d *DarwinKeyboard) PostKeyToWindow(hwnd uintptr, key Key, opts *PressOptions) error {
	return utils.WrapError(utils.ErrPlatformNotSupported,
		"PostKeyToWindow is only available on Windows; use Press on macOS "+
			"(note that it drives the real keyboard, so the target window must be focused)")
}

// IsSupported reports whether the key has a macOS key code mapping.
func IsSupported(key Key) bool {
	_, ok := darwinKeyCodes[key]
	return ok
}
