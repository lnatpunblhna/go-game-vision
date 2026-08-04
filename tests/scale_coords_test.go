package tests

import (
	stdimage "image"
	"runtime"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
)

// Offline regression tests for the pixel-to-click coordinate mapping.
//
// Background: captured images are in physical pixels while WindowInfo.Rect is in
// the platform's click coordinate space. On a Retina Mac those differ by 2x, so
// adding a match's pixel offset straight onto Rect.Min — as the code originally
// did — drifted every click by (scale-1)x the distance from the window origin.
// Measured on a 2880x1864 built-in display: a match at the centre of a 1073pt
// wide window resolved to (1435,1122) instead of (830,573).
//
// These tests need no window, no display and no permissions.

func TestScaleOrDefault(t *testing.T) {
	cases := []struct {
		name string
		info *capture.WindowInfo
		want float64
	}{
		// A hand-built or legacy WindowInfo leaves ScaleFactor at 0; degrading to
		// 1.0 is what stops a division by zero from throwing clicks to infinity.
		{"zero degrades to 1", &capture.WindowInfo{}, 1.0},
		{"negative degrades to 1", &capture.WindowInfo{ScaleFactor: -2}, 1.0},
		{"nil receiver degrades to 1", nil, 1.0},
		{"retina honoured", &capture.WindowInfo{ScaleFactor: 2.0}, 2.0},
		{"fractional honoured", &capture.WindowInfo{ScaleFactor: 1.5}, 1.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.info.ScaleOrDefault(); got != tc.want {
				t.Errorf("ScaleOrDefault() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPixelToClickPoint(t *testing.T) {
	// Mirrors the real measurement from a Retina display: window at (294,92)
	// sized 1073x962 points, captured at 2146x1924 pixels.
	retina := &capture.WindowInfo{
		Rect:        stdimage.Rect(294, 92, 294+1073, 92+962),
		ScaleFactor: 2.0,
	}

	cases := []struct {
		name         string
		info         *capture.WindowInfo
		px, py       int
		wantX, wantY int
	}{
		{"retina origin maps to window origin", retina, 0, 0, 294, 92},
		// 2146/2 = 1073 pixels in is the centre of the captured image; it must
		// land on the centre of the window, not 1073 points past its origin.
		//
		// X lands on 831, not 830: the window is an odd 1073pt wide, so its true
		// centre is 536.5pt and the half-point rounds up. A 1pt bias on an odd
		// dimension is inherent to integer click coordinates and harmless — the
		// pre-fix bug was 600pt, not 1pt.
		{"retina centre maps to window centre", retina, 1073, 962, 294 + 537, 92 + 481},
		{"retina far corner stays inside window", retina, 2146, 1924, 294 + 1073, 92 + 962},
		// Scale 1.0 must behave exactly like the original plain addition, so
		// Windows behaviour is unchanged by the fix.
		{
			"unscaled is plain translation",
			&capture.WindowInfo{Rect: stdimage.Rect(100, 200, 900, 800), ScaleFactor: 1.0},
			50, 60, 150, 260,
		},
		// Unset scale must also behave like plain addition rather than exploding.
		{
			"unset scale is plain translation",
			&capture.WindowInfo{Rect: stdimage.Rect(100, 200, 900, 800)},
			50, 60, 150, 260,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.info.PixelToClickPoint(tc.px, tc.py)
			if got.X != tc.wantX || got.Y != tc.wantY {
				t.Errorf("PixelToClickPoint(%d,%d) = (%d,%d), want (%d,%d)",
					tc.px, tc.py, got.X, got.Y, tc.wantX, tc.wantY)
			}
		})
	}
}

// TestPixelToClickPointStaysInWindow is the property that actually matters for
// automation: a match anywhere inside the captured image must map to a click
// inside the window. The pre-fix code violated this for every scale > 1.
func TestPixelToClickPointStaysInWindow(t *testing.T) {
	for _, scale := range []float64{1.0, 1.5, 2.0, 3.0} {
		info := &capture.WindowInfo{
			Rect:        stdimage.Rect(500, 300, 500+800, 300+600),
			ScaleFactor: scale,
		}
		imgW := int(800 * scale)
		imgH := int(600 * scale)

		for _, frac := range []float64{0, 0.25, 0.5, 0.75, 1.0} {
			px := int(float64(imgW) * frac)
			py := int(float64(imgH) * frac)
			got := info.PixelToClickPoint(px, py)

			if got.X < info.Rect.Min.X || got.X > info.Rect.Max.X ||
				got.Y < info.Rect.Min.Y || got.Y > info.Rect.Max.Y {
				t.Errorf("scale %.1f: pixel (%d,%d) mapped to (%d,%d), outside window %v",
					scale, px, py, got.X, got.Y, info.Rect)
			}
		}
	}
}

func TestMatchResultToScreenCoordinates(t *testing.T) {
	info := &capture.WindowInfo{
		Rect:        stdimage.Rect(294, 92, 294+1073, 92+962),
		ScaleFactor: 2.0,
	}
	m := &ggvimage.MatchResult{
		Location:    stdimage.Point{X: 400, Y: 300},
		BoundingBox: stdimage.Rect(400, 300, 500, 360),
	}

	// 400px / 2.0 = 200pt past the window origin.
	got := m.ToScreenCoordinates(info)
	if got.X != 294+200 || got.Y != 92+150 {
		t.Errorf("ToScreenCoordinates() = (%d,%d), want (%d,%d)",
			got.X, got.Y, 294+200, 92+150)
	}

	box := m.ToScreenBoundingBox(info)
	wantBox := stdimage.Rect(294+200, 92+150, 294+250, 92+180)
	if box != wantBox {
		t.Errorf("ToScreenBoundingBox() = %v, want %v", box, wantBox)
	}

	// The centre of the box is (450,330) in pixels -> (225,165) in points.
	center := m.ToScreenCenter(info)
	if center.X != 294+225 || center.Y != 92+165 {
		t.Errorf("ToScreenCenter() = (%d,%d), want (%d,%d)",
			center.X, center.Y, 294+225, 92+165)
	}
}

// TestCenterInPixelsFallsBackToLocation covers comparison methods that report a
// similarity score without a bounding box (histogram, SSIM), where the zero
// Rectangle would otherwise produce a (0,0) centre.
func TestCenterInPixelsFallsBackToLocation(t *testing.T) {
	m := &ggvimage.MatchResult{Location: stdimage.Point{X: 42, Y: 24}}
	if got := m.CenterInPixels(); got.X != 42 || got.Y != 24 {
		t.Errorf("CenterInPixels() = %v, want (42,24)", got)
	}
}

// TestDPIAwarenessIsIdempotent checks the platform helper can be called freely
// from any entry point without tripping over its sync.Once.
func TestDPIAwarenessIsIdempotent(t *testing.T) {
	first := platform.DPIAwareness()
	for i := 0; i < 3; i++ {
		platform.EnsureDPIAware()
	}
	if again := platform.DPIAwareness(); again != first {
		t.Errorf("DPIAwareness() changed across calls: %q then %q", first, again)
	}

	// On macOS there is no process-wide DPI opt-in to make.
	if runtime.GOOS == "darwin" && first != platform.DPIAwarenessUnsupported {
		t.Errorf("on darwin want %q, got %q", platform.DPIAwarenessUnsupported, first)
	}
}
