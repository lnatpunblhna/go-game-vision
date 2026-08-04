package mouse

import (
	"image"

	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Drag and scroll, provided as free functions rather than MouseClicker methods so
// that adding them does not break external implementations of that interface.

// DragOptions tunes a drag gesture.
type DragOptions struct {
	// Button is the button held during the drag. Defaults to LeftButton.
	Button MouseButton

	// Steps is how many intermediate move events to emit between the endpoints.
	// Zero means DefaultDragSteps.
	//
	// Intermediate moves are not decoration: a drag implemented as
	// down-at-A/up-at-B is frequently ignored, because applications track drags by
	// watching motion while the button is held and never see any. Games that
	// implement inertia or drag thresholds need the path, not just the endpoints.
	Steps int

	// StepDelayMS is the pause between intermediate moves. Zero means
	// DefaultDragStepDelayMS.
	StepDelayMS int

	// HoldMS is how long to wait after pressing the button before moving, and
	// again before releasing. Zero means DefaultDragHoldMS.
	//
	// Applications with a drag threshold or a press animation need a moment
	// between press and motion, or they treat the whole gesture as a click.
	HoldMS int
}

// Defaults for DragOptions.
const (
	DefaultDragSteps       = 20
	DefaultDragStepDelayMS = 8
	DefaultDragHoldMS      = 40
)

func (o *DragOptions) button() MouseButton {
	if o == nil {
		return LeftButton
	}
	return o.Button
}

func (o *DragOptions) steps() int {
	if o == nil || o.Steps <= 0 {
		return DefaultDragSteps
	}
	return o.Steps
}

func (o *DragOptions) stepDelayMS() int {
	if o == nil || o.StepDelayMS <= 0 {
		return DefaultDragStepDelayMS
	}
	return o.StepDelayMS
}

func (o *DragOptions) holdMS() int {
	if o == nil || o.HoldMS <= 0 {
		return DefaultDragHoldMS
	}
	return o.HoldMS
}

// ScrollOptions tunes a scroll gesture.
type ScrollOptions struct {
	// Horizontal scrolls sideways instead of vertically.
	Horizontal bool

	// StepDelayMS is the pause between scroll units. Zero means
	// DefaultScrollStepDelayMS.
	//
	// Scrolling many units in one burst is often coalesced into far less movement
	// than requested, so a small gap per unit makes the distance predictable.
	StepDelayMS int
}

// DefaultScrollStepDelayMS is the pause between scroll units.
const DefaultScrollStepDelayMS = 15

func (o *ScrollOptions) horizontal() bool {
	return o != nil && o.Horizontal
}

func (o *ScrollOptions) stepDelayMS() int {
	if o == nil || o.StepDelayMS <= 0 {
		return DefaultScrollStepDelayMS
	}
	return o.StepDelayMS
}

// Drag presses the button at from, moves to to, and releases.
//
// Coordinates are in the platform's click coordinate space, the same as
// BackgroundClick. On macOS this moves the real cursor, so it is subject to the
// same emergency-stop considerations as any other input (see pkg/safety).
func Drag(from, to image.Point, opts *DragOptions) error {
	platform.EnsureDPIAware()

	if err := ValidateCoordinates(from.X, from.Y); err != nil {
		return utils.WrapError(err, "drag start is off-screen")
	}
	if err := ValidateCoordinates(to.X, to.Y); err != nil {
		return utils.WrapError(err, "drag end is off-screen")
	}

	utils.Debug("dragging from (%d,%d) to (%d,%d) with %s button",
		from.X, from.Y, to.X, to.Y, opts.button().String())

	return dragPlatform(from, to, opts)
}

// DragTo drags from the current cursor position to a point.
func DragTo(to image.Point, opts *DragOptions) error {
	x, y, err := GetCursorPosition()
	if err != nil {
		return utils.WrapError(err, "failed to read the current cursor position")
	}
	return Drag(image.Point{X: x, Y: y}, to, opts)
}

// Scroll scrolls at the given point.
//
// A positive amount scrolls up (or left, when Horizontal is set); negative
// scrolls down (or right). The unit is one wheel notch, matching what a physical
// wheel click produces.
func Scroll(x, y int, amount int, opts *ScrollOptions) error {
	platform.EnsureDPIAware()

	if err := ValidateCoordinates(x, y); err != nil {
		return utils.WrapError(err, "scroll position is off-screen")
	}
	if amount == 0 {
		return nil
	}

	utils.Debug("scrolling %d notches at (%d,%d), horizontal=%v",
		amount, x, y, opts.horizontal())

	return scrollPlatform(x, y, amount, opts)
}

// ScrollAt is Scroll with a point.
func ScrollAt(at image.Point, amount int, opts *ScrollOptions) error {
	return Scroll(at.X, at.Y, amount, opts)
}

// interpolate returns the point a fraction t (0-1) along the segment from..to.
func interpolate(from, to image.Point, t float64) image.Point {
	return image.Point{
		X: from.X + int(float64(to.X-from.X)*t+0.5),
		Y: from.Y + int(float64(to.Y-from.Y)*t+0.5),
	}
}
