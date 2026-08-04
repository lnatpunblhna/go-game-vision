// Package keyboard provides synthetic keyboard input.
//
// Most games need this as much as they need the mouse — movement is WASD, skills
// are number keys, menus close with Escape — so a vision framework that can only
// click covers half the problem.
//
// Key identifies keys by *physical position*, not by the character they produce.
// That distinction matters: macOS virtual key codes are positional (kVK_ANSI_W is
// "the key where W sits on a US layout") while a character depends on the active
// layout, and games read positions. KeyW means the movement key whatever layout
// the user has active; use TypeString when you want characters instead.
package keyboard

import (
	"fmt"
	"sort"

	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
)

// Key identifies a key by physical position.
type Key int

// Key constants. Positional, following a US layout for naming.
const (
	KeyUnknown Key = iota

	// Letters
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ

	// Digits along the top row (not the numpad)
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9

	// Function keys
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	// Arrows
	KeyUp
	KeyDown
	KeyLeft
	KeyRight

	// Editing and navigation
	KeyReturn
	KeyEscape
	KeySpace
	KeyTab
	KeyBackspace
	KeyDelete
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown

	// Punctuation
	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyQuote
	KeyGrave
	KeyComma
	KeyPeriod
	KeySlash

	// Modifiers, as standalone keys for hold-and-release use
	KeyShift
	KeyControl
	KeyAlt
	KeyMeta // Command on macOS, Windows key on Windows
)

// String returns a readable key name.
func (k Key) String() string {
	if name, ok := keyNames[k]; ok {
		return name
	}
	return fmt.Sprintf("Key(%d)", int(k))
}

var keyNames = map[Key]string{
	KeyUnknown: "Unknown",
	KeyA:       "A", KeyB: "B", KeyC: "C", KeyD: "D", KeyE: "E", KeyF: "F",
	KeyG: "G", KeyH: "H", KeyI: "I", KeyJ: "J", KeyK: "K", KeyL: "L",
	KeyM: "M", KeyN: "N", KeyO: "O", KeyP: "P", KeyQ: "Q", KeyR: "R",
	KeyS: "S", KeyT: "T", KeyU: "U", KeyV: "V", KeyW: "W", KeyX: "X",
	KeyY: "Y", KeyZ: "Z",
	Key0: "0", Key1: "1", Key2: "2", Key3: "3", Key4: "4",
	Key5: "5", Key6: "6", Key7: "7", Key8: "8", Key9: "9",
	KeyF1: "F1", KeyF2: "F2", KeyF3: "F3", KeyF4: "F4", KeyF5: "F5", KeyF6: "F6",
	KeyF7: "F7", KeyF8: "F8", KeyF9: "F9", KeyF10: "F10", KeyF11: "F11", KeyF12: "F12",
	KeyUp: "Up", KeyDown: "Down", KeyLeft: "Left", KeyRight: "Right",
	KeyReturn: "Return", KeyEscape: "Escape", KeySpace: "Space", KeyTab: "Tab",
	KeyBackspace: "Backspace", KeyDelete: "Delete",
	KeyHome: "Home", KeyEnd: "End", KeyPageUp: "PageUp", KeyPageDown: "PageDown",
	KeyMinus: "Minus", KeyEqual: "Equal",
	KeyLeftBracket: "LeftBracket", KeyRightBracket: "RightBracket",
	KeyBackslash: "Backslash", KeySemicolon: "Semicolon", KeyQuote: "Quote",
	KeyGrave: "Grave", KeyComma: "Comma", KeyPeriod: "Period", KeySlash: "Slash",
	KeyShift: "Shift", KeyControl: "Control", KeyAlt: "Alt", KeyMeta: "Meta",
}

// Modifier is a bitmask of held modifier keys.
type Modifier int

// Modifier flags.
const (
	ModNone  Modifier = 0
	ModShift Modifier = 1 << iota
	ModControl
	ModAlt
	// ModMeta is Command on macOS and the Windows key on Windows.
	ModMeta
)

// Has reports whether the mask includes m.
func (mod Modifier) Has(m Modifier) bool { return mod&m != 0 }

// String returns a readable modifier list.
func (mod Modifier) String() string {
	if mod == ModNone {
		return "none"
	}
	out := ""
	appendName := func(name string) {
		if out != "" {
			out += "+"
		}
		out += name
	}
	if mod.Has(ModControl) {
		appendName("Ctrl")
	}
	if mod.Has(ModAlt) {
		appendName("Alt")
	}
	if mod.Has(ModShift) {
		appendName("Shift")
	}
	if mod.Has(ModMeta) {
		appendName("Meta")
	}
	return out
}

// PressOptions tunes a key press.
type PressOptions struct {
	// HoldMS is how long to hold the key down, in milliseconds. Zero means
	// DefaultHoldMS.
	//
	// Not cosmetic: many games sample input per frame, so a key that goes down and
	// up within the same frame is never observed. A few tens of milliseconds is
	// enough to span a frame at any normal refresh rate.
	HoldMS int

	// Modifiers are held for the duration of the press.
	Modifiers Modifier
}

// DefaultHoldMS is how long a key is held when unspecified.
const DefaultHoldMS = 30

func (o *PressOptions) holdMS() int {
	if o == nil || o.HoldMS <= 0 {
		return DefaultHoldMS
	}
	return o.HoldMS
}

func (o *PressOptions) modifiers() Modifier {
	if o == nil {
		return ModNone
	}
	return o.Modifiers
}

// Keyboard sends synthetic keyboard input.
type Keyboard interface {
	// KeyDown presses a key and leaves it held.
	//
	// Pair every KeyDown with a KeyUp. A leaked KeyDown leaves the key stuck down
	// system-wide — the user's next keystroke arrives with a phantom modifier, or
	// their character keeps walking — and nothing releases it when the process
	// exits.
	KeyDown(key Key) error

	// KeyUp releases a key.
	KeyUp(key Key) error

	// Press presses and releases a key, holding any modifiers around it.
	Press(key Key, opts *PressOptions) error

	// TypeString types a run of text.
	//
	// Character-oriented rather than positional, so it depends on the active
	// keyboard layout. Fine for chat and search boxes; not the way to drive
	// movement keys.
	TypeString(text string) error

	// PostKeyToWindow delivers a key to a specific window without activating it.
	//
	// Windows only; returns ErrPlatformNotSupported on macOS, which has no
	// equivalent public API. Many games also ignore it, because they read input
	// through DirectInput or raw device state rather than the window message
	// queue — test against the actual target before relying on it.
	PostKeyToWindow(hwnd uintptr, key Key, opts *PressOptions) error
}

// New creates a platform-specific Keyboard.
func New() Keyboard {
	platform.EnsureDPIAware()
	return newPlatformKeyboard()
}

// Press is a convenience wrapper for a single key press with no modifiers.
func Press(key Key) error {
	return New().Press(key, nil)
}

// PressWithModifiers presses a key while holding modifiers, e.g. Ctrl+S.
func PressWithModifiers(key Key, mods Modifier) error {
	return New().Press(key, &PressOptions{Modifiers: mods})
}

// TypeString types text using the active keyboard layout.
func TypeString(text string) error {
	return New().TypeString(text)
}

// Hold presses a key, runs fn, then releases the key — even if fn fails.
//
// This is the shape to use for "walk forward while looking for something": the
// deferred release is what stops an early return from leaving the key stuck down
// across the whole system.
func Hold(kb Keyboard, key Key, fn func() error) (err error) {
	if kb == nil {
		kb = New()
	}
	if err := kb.KeyDown(key); err != nil {
		return err
	}
	defer func() {
		if upErr := kb.KeyUp(key); upErr != nil && err == nil {
			err = upErr
		}
	}()
	return fn()
}

// AllKeys returns every named key, for coverage checks and for tools that
// enumerate the key space. Excludes KeyUnknown.
func AllKeys() []Key {
	keys := make([]Key, 0, len(keyNames))
	for k := range keyNames {
		if k == KeyUnknown {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
