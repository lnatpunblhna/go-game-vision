package tests

import (
	stdimage "image"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/automation"
	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
)

// Offline tests for the frame comparison behind WaitStable.

// flatFrame builds a uniform BGRA frame.
func flatFrame(w, h int, b, g, r, a byte) *capture.RawFrame {
	stride := w * 4
	pix := make([]byte, stride*h)
	for i := 0; i < len(pix); i += 4 {
		pix[i+0] = b
		pix[i+1] = g
		pix[i+2] = r
		pix[i+3] = a
	}
	return &capture.RawFrame{Pix: pix, Width: w, Height: h, Stride: stride}
}

func TestFrameDifferenceIdenticalFramesAreZero(t *testing.T) {
	a := flatFrame(128, 96, 10, 20, 30, 255)
	b := flatFrame(128, 96, 10, 20, 30, 255)

	diff, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 0)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if diff != 0 {
		t.Errorf("diff = %v, want 0 for identical frames", diff)
	}
}

func TestFrameDifferenceCompletelyDifferentIsOne(t *testing.T) {
	a := flatFrame(128, 96, 0, 0, 0, 255)
	b := flatFrame(128, 96, 255, 255, 255, 255)

	diff, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 8)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if diff != 1.0 {
		t.Errorf("diff = %v, want 1.0 for black vs white", diff)
	}
}

// TestFrameDifferenceSizeChangeIsFullyDifferent covers the resize case: there is
// no meaningful per-pixel comparison across geometries, and reporting "identical"
// would make WaitStable declare a resizing window stable.
func TestFrameDifferenceSizeChangeIsFullyDifferent(t *testing.T) {
	a := flatFrame(128, 96, 10, 20, 30, 255)
	b := flatFrame(64, 96, 10, 20, 30, 255)

	diff, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 0)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if diff != 1.0 {
		t.Errorf("diff = %v, want 1.0 when frame sizes differ", diff)
	}
}

// TestFrameDifferenceChannelDeltaAbsorbsNoise is why the tolerance exists: video
// compression and dithering make a static screen fluctuate by a few levels, and
// without a delta WaitStable would never be satisfied.
func TestFrameDifferenceChannelDeltaAbsorbsNoise(t *testing.T) {
	a := flatFrame(128, 96, 100, 100, 100, 255)
	b := flatFrame(128, 96, 104, 103, 102, 255) // within 8 on every channel

	withDelta, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 8)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if withDelta != 0 {
		t.Errorf("diff with delta 8 = %v, want 0 (noise should be absorbed)", withDelta)
	}

	withoutDelta, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 0)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if withoutDelta == 0 {
		t.Error("diff with delta 0 = 0, want > 0 (a real difference was ignored)")
	}
}

// TestFrameDifferenceIgnoresAlpha pins the documented decision to skip alpha:
// captured windows carry alpha that tracks compositing rather than content.
func TestFrameDifferenceIgnoresAlpha(t *testing.T) {
	a := flatFrame(128, 96, 50, 60, 70, 255)
	b := flatFrame(128, 96, 50, 60, 70, 0) // identical colour, opposite alpha

	diff, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 0)
	if err != nil {
		t.Fatalf("FrameDifference: %v", err)
	}
	if diff != 0 {
		t.Errorf("diff = %v, want 0 (alpha must not count as a change)", diff)
	}
}

// TestFrameDifferenceROIRestrictsComparison is the practical escape hatch: a clock
// or idle animation elsewhere on screen would otherwise keep the whole frame
// permanently unstable.
func TestFrameDifferenceROIRestrictsComparison(t *testing.T) {
	w, h := 200, 200
	a := flatFrame(w, h, 0, 0, 0, 255)
	b := flatFrame(w, h, 0, 0, 0, 255)

	// Change only the bottom-right quadrant of b.
	for y := 100; y < h; y++ {
		for x := 100; x < w; x++ {
			o := y*b.Stride + x*4
			b.Pix[o+0] = 255
			b.Pix[o+1] = 255
			b.Pix[o+2] = 255
		}
	}

	// A ROI over the untouched top-left quadrant sees no change.
	quiet, err := automation.FrameDifference(a, b, stdimage.Rect(0, 0, 100, 100), 8)
	if err != nil {
		t.Fatalf("FrameDifference (quiet ROI): %v", err)
	}
	if quiet != 0 {
		t.Errorf("quiet ROI diff = %v, want 0", quiet)
	}

	// A ROI over the changed quadrant sees a total change.
	busy, err := automation.FrameDifference(a, b, stdimage.Rect(100, 100, 200, 200), 8)
	if err != nil {
		t.Fatalf("FrameDifference (busy ROI): %v", err)
	}
	if busy != 1.0 {
		t.Errorf("busy ROI diff = %v, want 1.0", busy)
	}

	// The whole frame sits in between.
	full, err := automation.FrameDifference(a, b, stdimage.Rectangle{}, 8)
	if err != nil {
		t.Fatalf("FrameDifference (full): %v", err)
	}
	if full <= 0 || full >= 1 {
		t.Errorf("full-frame diff = %v, want strictly between 0 and 1", full)
	}
}

func TestFrameDifferenceRejectsBadInput(t *testing.T) {
	good := flatFrame(64, 64, 1, 2, 3, 255)

	if _, err := automation.FrameDifference(nil, good, stdimage.Rectangle{}, 0); err == nil {
		t.Error("expected an error for a nil first frame")
	}
	if _, err := automation.FrameDifference(good, nil, stdimage.Rectangle{}, 0); err == nil {
		t.Error("expected an error for a nil second frame")
	}

	// ROI entirely outside the frame.
	if _, err := automation.FrameDifference(good, good, stdimage.Rect(500, 500, 600, 600), 0); err == nil {
		t.Error("expected an error for a ROI outside the frame")
	}

	// Buffer shorter than the declared geometry.
	truncated := &capture.RawFrame{
		Pix:    make([]byte, 16),
		Width:  64,
		Height: 64,
		Stride: 64 * 4,
	}
	if _, err := automation.FrameDifference(truncated, truncated, stdimage.Rectangle{}, 0); err == nil {
		t.Error("expected an error for a truncated frame buffer")
	}
}
