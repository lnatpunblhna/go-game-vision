package tests

import (
	stdimage "image"
	"runtime"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/keyboard"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Offline keyboard tests.
//
// Deliberately no test that actually presses keys: synthetic input goes to
// whatever window has focus, which during a test run is the user's terminal or
// editor. A test that typed into it would corrupt whatever they were doing, and on
// CI it would type into nothing. What can be verified offline is the mapping and
// option handling, which is where the bugs actually are.

// TestAllKeysHavePlatformMapping is the test worth having: a Key with no keycode
// mapping compiles fine and only fails at the moment a script tries to press it,
// which in practice means mid-run against a live game.
func TestAllKeysHavePlatformMapping(t *testing.T) {
	keys := keyboard.AllKeys()
	if len(keys) < 60 {
		t.Fatalf("AllKeys returned only %d keys; the table looks truncated", len(keys))
	}

	var missing []string
	for _, k := range keys {
		if !keyboard.IsSupported(k) {
			missing = append(missing, k.String())
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d keys have no platform key code mapping: %v", len(missing), missing)
	}
}

func TestKeyUnknownIsNotSupported(t *testing.T) {
	if keyboard.IsSupported(keyboard.KeyUnknown) {
		t.Error("KeyUnknown should not have a key code mapping")
	}
	// Nor should an out-of-range value.
	if keyboard.IsSupported(keyboard.Key(9999)) {
		t.Error("an out-of-range Key should not be reported as supported")
	}
}

func TestKeyString(t *testing.T) {
	cases := map[keyboard.Key]string{
		keyboard.KeyA:       "A",
		keyboard.KeyZ:       "Z",
		keyboard.Key0:       "0",
		keyboard.Key9:       "9",
		keyboard.KeyF1:      "F1",
		keyboard.KeyF12:     "F12",
		keyboard.KeyEscape:  "Escape",
		keyboard.KeySpace:   "Space",
		keyboard.KeyReturn:  "Return",
		keyboard.KeyUp:      "Up",
		keyboard.KeyMeta:    "Meta",
		keyboard.KeyUnknown: "Unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Key.String() = %q, want %q", got, want)
		}
	}

	// An unnamed value must still stringify usefully rather than as empty.
	if got := keyboard.Key(9999).String(); got == "" {
		t.Error("an unnamed Key stringified to empty")
	}
}

// TestAllKeysAreDistinct guards against a copy-paste slip in the constant block
// giving two keys the same value, which would make them silently interchangeable.
func TestAllKeysAreDistinct(t *testing.T) {
	seen := map[keyboard.Key]bool{}
	names := map[string]bool{}
	for _, k := range keyboard.AllKeys() {
		if seen[k] {
			t.Errorf("duplicate key value %d (%s)", int(k), k)
		}
		seen[k] = true

		name := k.String()
		if names[name] {
			t.Errorf("duplicate key name %q", name)
		}
		names[name] = true
	}
}

func TestModifierHasAndString(t *testing.T) {
	if keyboard.ModNone.Has(keyboard.ModShift) {
		t.Error("ModNone should not contain ModShift")
	}

	combo := keyboard.ModControl | keyboard.ModShift
	if !combo.Has(keyboard.ModControl) {
		t.Error("combo should contain ModControl")
	}
	if !combo.Has(keyboard.ModShift) {
		t.Error("combo should contain ModShift")
	}
	if combo.Has(keyboard.ModAlt) {
		t.Error("combo should not contain ModAlt")
	}
	if combo.Has(keyboard.ModMeta) {
		t.Error("combo should not contain ModMeta")
	}

	if got := keyboard.ModNone.String(); got != "none" {
		t.Errorf("ModNone.String() = %q, want %q", got, "none")
	}
	if got := combo.String(); got != "Ctrl+Shift" {
		t.Errorf("combo.String() = %q, want %q", got, "Ctrl+Shift")
	}

	all := keyboard.ModControl | keyboard.ModAlt | keyboard.ModShift | keyboard.ModMeta
	if got := all.String(); got != "Ctrl+Alt+Shift+Meta" {
		t.Errorf("all.String() = %q, want %q", got, "Ctrl+Alt+Shift+Meta")
	}
}

// TestModifiersAreDistinctBits catches the classic iota-with-shift mistake, where
// two flags collapse onto the same bit and become indistinguishable.
func TestModifiersAreDistinctBits(t *testing.T) {
	mods := []keyboard.Modifier{
		keyboard.ModShift, keyboard.ModControl, keyboard.ModAlt, keyboard.ModMeta,
	}
	for i, a := range mods {
		if a == 0 {
			t.Errorf("modifier %d is zero", i)
		}
		// Each must be a single bit.
		if a&(a-1) != 0 {
			t.Errorf("modifier %d (%d) is not a single bit", i, int(a))
		}
		for j, b := range mods {
			if i != j && a == b {
				t.Errorf("modifiers %d and %d share the value %d", i, j, int(a))
			}
		}
	}
}

// TestKeyboardRejectsUnmappableKey checks the failure is an error rather than a
// silently dropped keystroke — a script that thinks it pressed a key and did not
// is far harder to debug than one that says so.
func TestKeyboardRejectsUnmappableKey(t *testing.T) {
	kb := keyboard.New()
	bogus := keyboard.Key(9999)

	if err := kb.KeyDown(bogus); err == nil {
		t.Error("KeyDown accepted an unmappable key")
	}
	if err := kb.KeyUp(bogus); err == nil {
		t.Error("KeyUp accepted an unmappable key")
	}
	if err := kb.Press(bogus, nil); err == nil {
		t.Error("Press accepted an unmappable key")
	}
}

// TestPostKeyToWindowUnsupportedOnDarwin pins the platform difference rather than
// letting it be discovered at runtime.
func TestPostKeyToWindowUnsupportedOnDarwin(t *testing.T) {
	if !isDarwin() {
		t.Skip("PostKeyToWindow is a Windows feature")
	}

	kb := keyboard.New()
	err := kb.PostKeyToWindow(12345, keyboard.KeyA, nil)
	if err == nil {
		t.Fatal("PostKeyToWindow should not be supported on macOS")
	}
	if !utils.IsError(err, utils.ErrPlatformNotSupported) {
		t.Errorf("error = %v, want ErrPlatformNotSupported", err)
	}
}

// TestHoldReleasesOnError is the leak that matters: a held key that is never
// released stays down system-wide, and nothing cleans it up when the process
// exits.
func TestHoldReleasesOnError(t *testing.T) {
	rec := &recordingKeyboard{}
	sentinel := utils.ErrInvalidParameter

	err := keyboard.Hold(rec, keyboard.KeyW, func() error {
		return sentinel
	})

	if err != sentinel {
		t.Errorf("Hold returned %v, want the callback's error", err)
	}
	if len(rec.events) != 2 {
		t.Fatalf("events = %v, want exactly a down and an up", rec.events)
	}
	if rec.events[0] != "down:W" || rec.events[1] != "up:W" {
		t.Errorf("events = %v, want [down:W up:W]", rec.events)
	}
}

func TestHoldReleasesOnSuccess(t *testing.T) {
	rec := &recordingKeyboard{}
	ran := false

	if err := keyboard.Hold(rec, keyboard.KeyShift, func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("Hold: %v", err)
	}

	if !ran {
		t.Error("the callback did not run")
	}
	if len(rec.events) != 2 || rec.events[1] != "up:Shift" {
		t.Errorf("events = %v, want the key released at the end", rec.events)
	}
}

// TestHoldPropagatesKeyDownFailure checks a failed press does not run the body and
// does not attempt a release for a key that was never pressed.
func TestHoldPropagatesKeyDownFailure(t *testing.T) {
	rec := &recordingKeyboard{downErr: utils.ErrPlatformNotSupported}
	ran := false

	err := keyboard.Hold(rec, keyboard.KeyW, func() error {
		ran = true
		return nil
	})

	if err == nil {
		t.Error("Hold returned nil despite KeyDown failing")
	}
	if ran {
		t.Error("the callback ran even though the key was never pressed")
	}
	for _, e := range rec.events {
		if e == "up:W" {
			t.Error("Hold released a key it never successfully pressed")
		}
	}
}

// recordingKeyboard records calls instead of sending real input.
type recordingKeyboard struct {
	events  []string
	downErr error
	upErr   error
}

func (r *recordingKeyboard) KeyDown(key keyboard.Key) error {
	if r.downErr != nil {
		return r.downErr
	}
	r.events = append(r.events, "down:"+key.String())
	return nil
}

func (r *recordingKeyboard) KeyUp(key keyboard.Key) error {
	if r.upErr != nil {
		return r.upErr
	}
	r.events = append(r.events, "up:"+key.String())
	return nil
}

func (r *recordingKeyboard) Press(key keyboard.Key, opts *keyboard.PressOptions) error {
	r.events = append(r.events, "press:"+key.String())
	return nil
}

func (r *recordingKeyboard) TypeString(text string) error {
	r.events = append(r.events, "type:"+text)
	return nil
}

func (r *recordingKeyboard) PostKeyToWindow(hwnd uintptr, key keyboard.Key, opts *keyboard.PressOptions) error {
	r.events = append(r.events, "post:"+key.String())
	return nil
}

// --- gestures ---------------------------------------------------------------

// TestDragValidatesCoordinates checks off-screen endpoints are rejected before any
// button is pressed — a drag that starts by pressing at an invalid point and then
// fails would leave the button held.
func TestDragValidatesCoordinates(t *testing.T) {
	far := stdimage.Point{X: 1 << 20, Y: 1 << 20}
	ok := stdimage.Point{X: 10, Y: 10}

	if err := mouse.Drag(far, ok, nil); err == nil {
		t.Error("Drag accepted an off-screen start point")
	}
	if err := mouse.Drag(ok, far, nil); err == nil {
		t.Error("Drag accepted an off-screen end point")
	}
}

func TestScrollValidatesCoordinatesAndZero(t *testing.T) {
	if err := mouse.Scroll(1<<20, 1<<20, 3, nil); err == nil {
		t.Error("Scroll accepted an off-screen position")
	}
	// A zero-amount scroll is a no-op, not an error: it lets callers pass a
	// computed delta without special-casing zero.
	if err := mouse.Scroll(10, 10, 0, nil); err != nil {
		t.Errorf("Scroll with amount 0 returned %v, want nil", err)
	}
}

// isDarwin reports whether the tests are running on macOS.
func isDarwin() bool { return runtime.GOOS == "darwin" }
