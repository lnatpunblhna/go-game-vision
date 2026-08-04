package tests

import (
	stdimage "image"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
)

// Offline tests for HSV colour detection. Synthetic frames only — no window,
// display or permissions needed.

// bgraFrame builds a frame and lets the caller paint into it via a setter.
func bgraFrame(w, h int) (*capture.RawFrame, func(x, y int, b, g, r byte)) {
	stride := w * 4
	pix := make([]byte, stride*h)
	// Start fully opaque; alpha is not part of colour detection but a zero alpha
	// would be a misleading frame to reason about.
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 255
	}
	frame := &capture.RawFrame{Pix: pix, Width: w, Height: h, Stride: stride}

	set := func(x, y int, b, g, r byte) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		o := y*stride + x*4
		pix[o+0] = b
		pix[o+1] = g
		pix[o+2] = r
		pix[o+3] = 255
	}
	return frame, set
}

func fillRect(set func(x, y int, b, g, r byte), rect stdimage.Rectangle, b, g, r byte) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			set(x, y, b, g, r)
		}
	}
}

func TestColorRangeWraps(t *testing.T) {
	// Red is the case that forces wraparound support: it occupies both ends of
	// the hue circle, so it cannot be one ascending interval.
	if !ggvimage.RangeRed.Wraps() {
		t.Error("RangeRed should wrap the hue boundary")
	}
	if ggvimage.RangeGreen.Wraps() {
		t.Error("RangeGreen should not wrap")
	}
	if ggvimage.RangeBlue.Wraps() {
		t.Error("RangeBlue should not wrap")
	}
}

func TestColorDetectorRatioOnUniformFrames(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	cases := []struct {
		name    string
		b, g, r byte
		match   ggvimage.ColorRange
		wantAll bool
	}{
		// Pure red is the wraparound case: hue 0, which a naive 170-10 threshold
		// implemented as a single interval would miss entirely.
		{"pure red vs red range", 0, 0, 255, ggvimage.RangeRed, true},
		{"pure green vs green range", 0, 255, 0, ggvimage.RangeGreen, true},
		{"pure blue vs blue range", 255, 0, 0, ggvimage.RangeBlue, true},
		{"pure red vs green range", 0, 0, 255, ggvimage.RangeGreen, false},
		{"pure green vs red range", 0, 255, 0, ggvimage.RangeRed, false},
		{"white vs white range", 255, 255, 255, ggvimage.RangeWhite, true},
		{"black vs black range", 0, 0, 0, ggvimage.RangeBlack, true},
		// Saturation floors exist precisely so greys do not register as colour.
		{"white vs red range", 255, 255, 255, ggvimage.RangeRed, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame, set := bgraFrame(64, 48)
			fillRect(set, stdimage.Rect(0, 0, 64, 48), tc.b, tc.g, tc.r)

			ratio, err := d.Ratio(frame, stdimage.Rectangle{}, tc.match)
			if err != nil {
				t.Fatalf("Ratio: %v", err)
			}

			if tc.wantAll && ratio < 0.99 {
				t.Errorf("ratio = %.3f, want ~1.0 (colour should match %s)", ratio, tc.match.Name)
			}
			if !tc.wantAll && ratio > 0.01 {
				t.Errorf("ratio = %.3f, want ~0.0 (colour should not match %s)", ratio, tc.match.Name)
			}
		})
	}
}

func TestColorDetectorRatioIsProportional(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	// Half green, half black.
	frame, set := bgraFrame(100, 100)
	fillRect(set, stdimage.Rect(0, 0, 100, 50), 0, 255, 0)

	ratio, err := d.Ratio(frame, stdimage.Rectangle{}, ggvimage.RangeGreen)
	if err != nil {
		t.Fatalf("Ratio: %v", err)
	}
	if ratio < 0.45 || ratio > 0.55 {
		t.Errorf("ratio = %.3f, want ~0.5 for a half-green frame", ratio)
	}
}

func TestColorDetectorRatioRespectsROI(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	frame, set := bgraFrame(200, 100)
	// Green only in the left half.
	fillRect(set, stdimage.Rect(0, 0, 100, 100), 0, 255, 0)

	left, err := d.Ratio(frame, stdimage.Rect(0, 0, 100, 100), ggvimage.RangeGreen)
	if err != nil {
		t.Fatalf("Ratio(left): %v", err)
	}
	if left < 0.99 {
		t.Errorf("left ROI ratio = %.3f, want ~1.0", left)
	}

	right, err := d.Ratio(frame, stdimage.Rect(100, 0, 200, 100), ggvimage.RangeGreen)
	if err != nil {
		t.Fatalf("Ratio(right): %v", err)
	}
	if right > 0.01 {
		t.Errorf("right ROI ratio = %.3f, want ~0.0", right)
	}
}

// TestColorDetectorBarLevel is the health-bar case that motivates the whole file.
func TestColorDetectorBarLevel(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	for _, want := range []float64{0.0, 0.25, 0.5, 0.75, 1.0} {
		barRect := stdimage.Rect(20, 40, 220, 60) // 200x20 bar
		frame, set := bgraFrame(300, 100)

		// Empty portion: dark grey, as a real bar's track usually is.
		fillRect(set, barRect, 40, 40, 40)
		// Filled portion: green, from the left.
		filledWidth := int(float64(barRect.Dx()) * want)
		if filledWidth > 0 {
			fillRect(set, stdimage.Rect(barRect.Min.X, barRect.Min.Y,
				barRect.Min.X+filledWidth, barRect.Max.Y), 0, 200, 0)
		}

		got, err := d.BarLevel(frame, barRect, ggvimage.RangeGreen, nil)
		if err != nil {
			t.Fatalf("BarLevel at %.2f: %v", want, err)
		}
		if diff := got - want; diff > 0.02 || diff < -0.02 {
			t.Errorf("BarLevel = %.3f, want %.3f", got, want)
		}
	}
}

// TestColorDetectorBarLevelToleratesBorder covers why BarLevel counts lines
// instead of pixels: a bar with a border leaves part of every column unmatched, so
// a plain pixel ratio understates the fill.
func TestColorDetectorBarLevelToleratesBorder(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	barRect := stdimage.Rect(10, 10, 210, 40) // 200x30
	frame, set := bgraFrame(250, 60)

	// Dark track everywhere, a 4px light border top and bottom, green fill to 60%
	// in between.
	fillRect(set, barRect, 40, 40, 40)
	fillRect(set, stdimage.Rect(barRect.Min.X, barRect.Min.Y, barRect.Max.X, barRect.Min.Y+4), 200, 200, 200)
	fillRect(set, stdimage.Rect(barRect.Min.X, barRect.Max.Y-4, barRect.Max.X, barRect.Max.Y), 200, 200, 200)
	fillRect(set, stdimage.Rect(barRect.Min.X, barRect.Min.Y+4, barRect.Min.X+120, barRect.Max.Y-4), 0, 200, 0)

	level, err := d.BarLevel(frame, barRect, ggvimage.RangeGreen, nil)
	if err != nil {
		t.Fatalf("BarLevel: %v", err)
	}
	if level < 0.55 || level > 0.65 {
		t.Errorf("BarLevel = %.3f, want ~0.60 despite the border", level)
	}

	// The plain pixel ratio is materially lower — that gap is the reason BarLevel
	// exists rather than callers using Ratio for bars.
	ratio, err := d.Ratio(frame, barRect, ggvimage.RangeGreen)
	if err != nil {
		t.Fatalf("Ratio: %v", err)
	}
	if ratio >= level {
		t.Errorf("expected the pixel ratio (%.3f) to understate the bar level (%.3f)", ratio, level)
	}
}

func TestColorDetectorBarLevelOrientations(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	const size = 100
	cases := []struct {
		orientation ggvimage.Orientation
		fill        stdimage.Rectangle
	}{
		// 30% filled from each respective edge.
		{ggvimage.LeftToRight, stdimage.Rect(0, 0, 30, size)},
		{ggvimage.RightToLeft, stdimage.Rect(70, 0, size, size)},
		{ggvimage.TopToBottom, stdimage.Rect(0, 0, size, 30)},
		{ggvimage.BottomToTop, stdimage.Rect(0, 70, size, size)},
	}

	for _, tc := range cases {
		t.Run(tc.orientation.String(), func(t *testing.T) {
			frame, set := bgraFrame(size, size)
			fillRect(set, stdimage.Rect(0, 0, size, size), 40, 40, 40)
			fillRect(set, tc.fill, 0, 200, 0)

			level, err := d.BarLevel(frame, stdimage.Rectangle{}, ggvimage.RangeGreen,
				&ggvimage.BarLevelOptions{Orientation: tc.orientation})
			if err != nil {
				t.Fatalf("BarLevel: %v", err)
			}
			if level < 0.28 || level > 0.32 {
				t.Errorf("BarLevel = %.3f, want ~0.30 for %s", level, tc.orientation)
			}
		})
	}
}

func TestColorDetectorFindBlobs(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	frame, set := bgraFrame(300, 200)
	// Three red squares of decreasing size, plus one speck below minArea.
	fillRect(set, stdimage.Rect(10, 10, 70, 70), 0, 0, 255)     // 60x60 = 3600
	fillRect(set, stdimage.Rect(150, 20, 190, 60), 0, 0, 255)   // 40x40 = 1600
	fillRect(set, stdimage.Rect(220, 120, 245, 145), 0, 0, 255) // 25x25 = 625
	fillRect(set, stdimage.Rect(100, 180, 103, 183), 0, 0, 255) // 3x3 = 9, noise

	blobs, err := d.FindBlobs(frame, stdimage.Rectangle{}, ggvimage.RangeRed, 100)
	if err != nil {
		t.Fatalf("FindBlobs: %v", err)
	}

	if len(blobs) != 3 {
		t.Fatalf("got %d blobs, want 3 (the 3x3 speck should be below minArea): %+v", len(blobs), blobs)
	}

	// Must be ordered largest first.
	for i := 1; i < len(blobs); i++ {
		if blobs[i-1].Area < blobs[i].Area {
			t.Errorf("blobs not sorted by area: %d before %d", blobs[i-1].Area, blobs[i].Area)
		}
	}

	// The largest blob's centre should be the centre of the 60x60 square.
	wantCenter := stdimage.Point{X: 40, Y: 40}
	if abs(blobs[0].Center.X-wantCenter.X) > 2 || abs(blobs[0].Center.Y-wantCenter.Y) > 2 {
		t.Errorf("largest blob centre = %v, want near %v", blobs[0].Center, wantCenter)
	}
}

// TestColorDetectorFindBlobsROICoordinates checks blob coordinates come back
// frame-relative, so they can be clicked without the caller undoing a ROI offset.
func TestColorDetectorFindBlobsROICoordinates(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	frame, set := bgraFrame(400, 300)
	fillRect(set, stdimage.Rect(250, 150, 300, 200), 0, 0, 255)

	roi := stdimage.Rect(200, 100, 400, 300)
	blobs, err := d.FindBlobs(frame, roi, ggvimage.RangeRed, 100)
	if err != nil {
		t.Fatalf("FindBlobs: %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("got %d blobs, want 1", len(blobs))
	}

	wantCenter := stdimage.Point{X: 275, Y: 175}
	if abs(blobs[0].Center.X-wantCenter.X) > 2 || abs(blobs[0].Center.Y-wantCenter.Y) > 2 {
		t.Errorf("blob centre = %v, want frame-relative %v", blobs[0].Center, wantCenter)
	}
}

func TestColorDetectorSampleHSV(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	frame, set := bgraFrame(32, 32)
	// Pure green: OpenCV hue 60 (i.e. 120 degrees halved), full saturation/value.
	fillRect(set, stdimage.Rect(0, 0, 32, 32), 0, 255, 0)

	h, s, v, err := d.SampleHSV(frame, 16, 16)
	if err != nil {
		t.Fatalf("SampleHSV: %v", err)
	}
	if h < 55 || h > 65 {
		t.Errorf("hue = %d, want ~60 for pure green (OpenCV halves degrees)", h)
	}
	if s < 250 {
		t.Errorf("saturation = %d, want ~255 for a pure colour", s)
	}
	if v < 250 {
		t.Errorf("value = %d, want ~255 for full brightness", v)
	}

	if _, _, _, err := d.SampleHSV(frame, 100, 100); err == nil {
		t.Error("expected an error sampling outside the frame")
	}
}

func TestColorDetectorRejectsBadInput(t *testing.T) {
	d := ggvimage.NewColorDetector()
	defer d.Close()

	frame, _ := bgraFrame(64, 64)

	if _, err := d.Ratio(nil, stdimage.Rectangle{}, ggvimage.RangeRed); err == nil {
		t.Error("expected an error for a nil frame")
	}
	if _, err := d.Ratio(frame, stdimage.Rect(500, 500, 600, 600), ggvimage.RangeRed); err == nil {
		t.Error("expected an error for a ROI outside the frame")
	}
}

func TestOrientationString(t *testing.T) {
	cases := map[ggvimage.Orientation]string{
		ggvimage.LeftToRight:    "left-to-right",
		ggvimage.RightToLeft:    "right-to-left",
		ggvimage.BottomToTop:    "bottom-to-top",
		ggvimage.TopToBottom:    "top-to-bottom",
		ggvimage.Orientation(9): "unknown",
	}
	for o, want := range cases {
		if got := o.String(); got != want {
			t.Errorf("Orientation(%d).String() = %q, want %q", o, got, want)
		}
	}
}
