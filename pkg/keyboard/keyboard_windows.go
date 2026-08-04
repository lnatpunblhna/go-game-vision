//go:build windows

package keyboard

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	procSendInput   = user32.NewProc("SendInput")
	procPostMessage = user32.NewProc("PostMessageW")
	procMapVirtualK = user32.NewProc("MapVirtualKeyW")
)

// Windows input constants.
const (
	inputKeyboard = 1

	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004

	wmKeyDown = 0x0100
	wmKeyUp   = 0x0101
	wmChar    = 0x0102

	mapvkVkToVsc = 0
)

// Windows virtual key codes.
const (
	vkBack      = 0x08
	vkTab       = 0x09
	vkReturn    = 0x0D
	vkShift     = 0x10
	vkControl   = 0x11
	vkMenu      = 0x12 // Alt
	vkEscape    = 0x1B
	vkSpace     = 0x20
	vkPrior     = 0x21 // Page Up
	vkNext      = 0x22 // Page Down
	vkEnd       = 0x23
	vkHome      = 0x24
	vkLeft      = 0x25
	vkUp        = 0x26
	vkRight     = 0x27
	vkDown      = 0x28
	vkDelete    = 0x2E
	vkLWin      = 0x5B
	vkOEMMinus  = 0xBD
	vkOEMPlus   = 0xBB
	vkOEM1      = 0xBA // ;:
	vkOEM2      = 0xBF // /?
	vkOEM3      = 0xC0 // `~
	vkOEM4      = 0xDB // [{
	vkOEM5      = 0xDC // \|
	vkOEM6      = 0xDD // ]}
	vkOEM7      = 0xDE // '"
	vkOEMComma  = 0xBC
	vkOEMPeriod = 0xBE
)

// windowsKeyCodes maps Key to Windows virtual key codes.
//
// Letters and digits are contiguous in the VK space (VK_A == 'A' == 0x41), so
// those are computed rather than listed; everything else needs the table.
var windowsKeyCodes = map[Key]uint16{
	KeyF1: 0x70, KeyF2: 0x71, KeyF3: 0x72, KeyF4: 0x73,
	KeyF5: 0x74, KeyF6: 0x75, KeyF7: 0x76, KeyF8: 0x77,
	KeyF9: 0x78, KeyF10: 0x79, KeyF11: 0x7A, KeyF12: 0x7B,

	KeyUp: vkUp, KeyDown: vkDown, KeyLeft: vkLeft, KeyRight: vkRight,

	KeyReturn: vkReturn, KeyEscape: vkEscape, KeySpace: vkSpace, KeyTab: vkTab,
	KeyBackspace: vkBack, KeyDelete: vkDelete,
	KeyHome: vkHome, KeyEnd: vkEnd, KeyPageUp: vkPrior, KeyPageDown: vkNext,

	KeyMinus: vkOEMMinus, KeyEqual: vkOEMPlus,
	KeyLeftBracket: vkOEM4, KeyRightBracket: vkOEM6, KeyBackslash: vkOEM5,
	KeySemicolon: vkOEM1, KeyQuote: vkOEM7, KeyGrave: vkOEM3,
	KeyComma: vkOEMComma, KeyPeriod: vkOEMPeriod, KeySlash: vkOEM2,

	KeyShift: vkShift, KeyControl: vkControl, KeyAlt: vkMenu, KeyMeta: vkLWin,
}

// KEYBDINPUT matches the Win32 KEYBDINPUT structure.
type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT matches the Win32 INPUT structure for keyboard events.
//
// The padding reproduces the union layout: INPUT's union is 8-byte aligned on
// amd64, so the type field is followed by 4 bytes of padding before the payload.
type INPUT struct {
	Type uint32
	_    [4]byte
	Ki   KEYBDINPUT
	_    [8]byte // union is sized by MOUSEINPUT, which is larger than KEYBDINPUT
}

// WindowsKeyboard implements Keyboard on Windows.
type WindowsKeyboard struct{}

func newPlatformKeyboard() Keyboard {
	return &WindowsKeyboard{}
}

// keyCode resolves a Key to a Windows virtual key code.
func keyCode(key Key) (uint16, error) {
	// Letters: VK codes coincide with uppercase ASCII.
	if key >= KeyA && key <= KeyZ {
		return uint16(0x41 + int(key-KeyA)), nil
	}
	// Digits: VK codes coincide with ASCII digits.
	if key >= Key0 && key <= Key9 {
		return uint16(0x30 + int(key-Key0)), nil
	}
	if code, ok := windowsKeyCodes[key]; ok {
		return code, nil
	}
	return 0, utils.WrapError(utils.ErrInvalidParameter,
		fmt.Sprintf("key %s has no Windows key code mapping", key))
}

// modifierKeys expands a Modifier mask into the virtual keys to hold.
func modifierKeys(mod Modifier) []uint16 {
	var keys []uint16
	if mod.Has(ModControl) {
		keys = append(keys, vkControl)
	}
	if mod.Has(ModAlt) {
		keys = append(keys, vkMenu)
	}
	if mod.Has(ModShift) {
		keys = append(keys, vkShift)
	}
	if mod.Has(ModMeta) {
		keys = append(keys, vkLWin)
	}
	return keys
}

// sendKey posts one key event via SendInput.
//
// Sends the scan code alongside the virtual key: many games read input through
// DirectInput or raw input, which look at scan codes and ignore events that carry
// only a virtual key.
func sendKey(vk uint16, up bool) error {
	scan, _, _ := procMapVirtualK.Call(uintptr(vk), mapvkVkToVsc)

	var flags uint32
	if up {
		flags |= keyeventfKeyUp
	}

	input := INPUT{
		Type: inputKeyboard,
		Ki: KEYBDINPUT{
			WVk:     vk,
			WScan:   uint16(scan),
			DwFlags: flags,
		},
	}

	ret, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if ret != 1 {
		return utils.WrapError(err, "SendInput failed")
	}
	return nil
}

// sendUnicode posts one UTF-16 unit as a character event, bypassing key codes.
func sendUnicode(unit uint16, up bool) error {
	flags := uint32(keyeventfUnicode)
	if up {
		flags |= keyeventfKeyUp
	}

	input := INPUT{
		Type: inputKeyboard,
		Ki: KEYBDINPUT{
			WVk:     0,
			WScan:   unit,
			DwFlags: flags,
		},
	}

	ret, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if ret != 1 {
		return utils.WrapError(err, "SendInput (unicode) failed")
	}
	return nil
}

// KeyDown presses a key and leaves it held.
func (w *WindowsKeyboard) KeyDown(key Key) error {
	vk, err := keyCode(key)
	if err != nil {
		return err
	}
	if err := sendKey(vk, false); err != nil {
		return err
	}
	utils.Debug("key down: %s", key)
	return nil
}

// KeyUp releases a key.
func (w *WindowsKeyboard) KeyUp(key Key) error {
	vk, err := keyCode(key)
	if err != nil {
		return err
	}
	if err := sendKey(vk, true); err != nil {
		return err
	}
	utils.Debug("key up: %s", key)
	return nil
}

// Press presses and releases a key with modifiers held around it.
func (w *WindowsKeyboard) Press(key Key, opts *PressOptions) error {
	vk, err := keyCode(key)
	if err != nil {
		return err
	}

	mods := modifierKeys(opts.modifiers())

	// Press modifiers first, and release them in reverse order afterwards. The
	// releases run even on failure: leaving Ctrl or Alt stuck down affects every
	// subsequent keystroke system-wide, so it must not depend on the happy path.
	for _, mod := range mods {
		if err := sendKey(mod, false); err != nil {
			w.releaseModifiers(mods)
			return err
		}
	}
	defer w.releaseModifiers(mods)

	if err := sendKey(vk, false); err != nil {
		return err
	}
	time.Sleep(time.Duration(opts.holdMS()) * time.Millisecond)
	if err := sendKey(vk, true); err != nil {
		return err
	}

	utils.Debug("pressed %s with modifiers %s", key, opts.modifiers())
	return nil
}

// releaseModifiers releases held modifiers in reverse order, best effort.
func (w *WindowsKeyboard) releaseModifiers(mods []uint16) {
	for i := len(mods) - 1; i >= 0; i-- {
		if err := sendKey(mods[i], true); err != nil {
			utils.Warn("failed to release modifier 0x%02X: %v", mods[i], err)
		}
	}
}

// TypeString types text as unicode characters, independent of keyboard layout.
func (w *WindowsKeyboard) TypeString(text string) error {
	for _, unit := range utf16Encode(text) {
		if err := sendUnicode(unit, false); err != nil {
			return err
		}
		if err := sendUnicode(unit, true); err != nil {
			return err
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// PostKeyToWindow delivers a key to a window without activating it.
//
// Works only for applications that read input from the window message queue.
// Games using DirectInput or raw input ignore posted messages entirely, so verify
// against the real target rather than assuming.
func (w *WindowsKeyboard) PostKeyToWindow(hwnd uintptr, key Key, opts *PressOptions) error {
	if hwnd == 0 {
		return utils.WrapError(utils.ErrInvalidParameter, "window handle is zero")
	}
	vk, err := keyCode(key)
	if err != nil {
		return err
	}

	scan, _, _ := procMapVirtualK.Call(uintptr(vk), mapvkVkToVsc)

	// lParam layout for WM_KEYDOWN/WM_KEYUP: repeat count in bits 0-15, scan code
	// in 16-23. WM_KEYUP additionally sets bit 30 (previous state was down) and
	// bit 31 (transition state); omitting those makes some applications ignore the
	// release and treat the key as still held.
	lParamDown := uintptr(1) | (scan << 16)
	lParamUp := lParamDown | (1 << 30) | (1 << 31)

	ret, _, err := procPostMessage.Call(hwnd, wmKeyDown, uintptr(vk), lParamDown)
	if ret == 0 {
		return utils.WrapError(err, "PostMessage(WM_KEYDOWN) failed")
	}

	time.Sleep(time.Duration(opts.holdMS()) * time.Millisecond)

	ret, _, err = procPostMessage.Call(hwnd, wmKeyUp, uintptr(vk), lParamUp)
	if ret == 0 {
		return utils.WrapError(err, "PostMessage(WM_KEYUP) failed")
	}

	utils.Debug("posted %s to window 0x%X", key, hwnd)
	return nil
}

// PostTextToWindow posts text to a window as WM_CHAR messages, without activating
// it. Windows-only convenience beyond the Keyboard interface.
func (w *WindowsKeyboard) PostTextToWindow(hwnd uintptr, text string) error {
	if hwnd == 0 {
		return utils.WrapError(utils.ErrInvalidParameter, "window handle is zero")
	}
	for _, unit := range utf16Encode(text) {
		ret, _, err := procPostMessage.Call(hwnd, wmChar, uintptr(unit), 1)
		if ret == 0 {
			return utils.WrapError(err, "PostMessage(WM_CHAR) failed")
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// IsSupported reports whether the key has a Windows key code mapping.
func IsSupported(key Key) bool {
	_, err := keyCode(key)
	return err == nil
}
