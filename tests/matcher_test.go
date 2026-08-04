package tests

import (
	stdimage "image"
	"image/color"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Offline tests for the Mat hot path. These synthesise frames rather than
// capturing them, so they need no window, display or permissions — only OpenCV.

// synthFrame builds a RawFrame in BGRA with a distinctive patch drawn at
// (patchX, patchY), and returns the frame plus that patch as a standalone image.
//
// The patch is a deterministic gradient rather than a solid block: a solid patch
// on a solid background matches equally well at many offsets, which makes any
// assertion about the located position meaningless.
func synthFrame(w, h, patchX, patchY, patchW, patchH int) (*capture.RawFrame, *stdimage.RGBA) {
	stride := w * 4
	pix := make([]byte, stride*h)

	// Background: a slow diagonal ramp, distinct from the patch pattern.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*stride + x*4
			v := byte((x + y) % 64)
			pix[o+0] = v     // B
			pix[o+1] = v / 2 // G
			pix[o+2] = 20    // R
			pix[o+3] = 255   // A
		}
	}

	patch := stdimage.NewRGBA(stdimage.Rect(0, 0, patchW, patchH))
	for dy := 0; dy < patchH; dy++ {
		for dx := 0; dx < patchW; dx++ {
			r := byte(255 - dx*3%256)
			g := byte(dy * 5 % 256)
			b := byte((dx*7 + dy*11) % 256)

			patch.Set(dx, dy, color.RGBA{R: r, G: g, B: b, A: 255})

			x, y := patchX+dx, patchY+dy
			if x >= w || y >= h {
				continue
			}
			o := y*stride + x*4
			pix[o+0] = b
			pix[o+1] = g
			pix[o+2] = r
			pix[o+3] = 255
		}
	}

	frame := &capture.RawFrame{
		Pix:    pix,
		Width:  w,
		Height: h,
		Stride: stride,
		Window: capture.WindowInfo{
			Rect:        stdimage.Rect(100, 50, 100+w/2, 50+h/2),
			ScaleFactor: 2.0,
		},
	}
	return frame, patch
}

func TestMatcherFindsPatchAtCorrectLocation(t *testing.T) {
	frame, patch := synthFrame(600, 400, 220, 160, 60, 40)

	tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	got, err := m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.9})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	if got.Location.X != 220 || got.Location.Y != 160 {
		t.Errorf("Location = %v, want (220,160) (similarity %.4f)", got.Location, got.Similarity)
	}
	if got.Similarity < 0.99 {
		t.Errorf("similarity = %.4f, want >= 0.99 for an exact crop", got.Similarity)
	}
	wantBox := stdimage.Rect(220, 160, 280, 200)
	if got.BoundingBox != wantBox {
		t.Errorf("BoundingBox = %v, want %v", got.BoundingBox, wantBox)
	}
	if size := tpl.Size(); size.X != 60 || size.Y != 40 {
		t.Errorf("Template.Size() = %v, want (60,40)", size)
	}
}

// TestMatcherROIReturnsFrameCoordinates is the correctness property that makes
// ROI usable: restricting the search must not change the coordinate space of the
// result. If the ROI origin were not added back, every ROI match would produce a
// click offset by the ROI's position.
func TestMatcherROIReturnsFrameCoordinates(t *testing.T) {
	frame, patch := synthFrame(600, 400, 220, 160, 60, 40)

	tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	// A ROI that contains the patch but does not start at the frame origin.
	roi := stdimage.Rect(200, 140, 400, 300)
	got, err := m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.9, ROI: roi})
	if err != nil {
		t.Fatalf("Find with ROI: %v", err)
	}

	if got.Location.X != 220 || got.Location.Y != 160 {
		t.Errorf("ROI match Location = %v, want frame-relative (220,160)", got.Location)
	}
}

func TestMatcherROIExcludingTargetFindsNothing(t *testing.T) {
	frame, patch := synthFrame(600, 400, 220, 160, 60, 40)

	tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	// Search a region the patch is not in.
	roi := stdimage.Rect(400, 300, 600, 400)
	_, err = m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.9, ROI: roi})
	if err == nil {
		t.Fatal("expected no match when the ROI excludes the target")
	}
	if !utils.IsError(err, ggvimage.ErrNoMatch) {
		t.Errorf("expected ErrNoMatch, got %v", err)
	}
}

// TestMatcherROIIsClampedToFrame checks an out-of-range ROI is clamped rather
// than reaching OpenCV, where Region() on an out-of-bounds rectangle panics.
func TestMatcherROIIsClampedToFrame(t *testing.T) {
	frame, patch := synthFrame(600, 400, 220, 160, 60, 40)

	tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	// Extends well past the frame on all sides but still covers the patch.
	roi := stdimage.Rect(-500, -500, 5000, 5000)
	got, err := m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.9, ROI: roi})
	if err != nil {
		t.Fatalf("Find with oversized ROI: %v", err)
	}
	if got.Location.X != 220 || got.Location.Y != 160 {
		t.Errorf("Location = %v, want (220,160)", got.Location)
	}

	// A ROI entirely outside the frame is an error, not a panic.
	_, err = m.Find(frame, tpl, &ggvimage.MatchOptions{
		Threshold: 0.9,
		ROI:       stdimage.Rect(2000, 2000, 2100, 2100),
	})
	if err == nil {
		t.Error("expected an error for a ROI completely outside the frame")
	}
}

// TestMatcherFindAllSuppressesOverlaps guards the non-maximum suppression: a
// strong match produces a cluster of high scores around its true position, and
// without suppression each would be reported as a separate hit.
func TestMatcherFindAllSuppressesOverlaps(t *testing.T) {
	// Two copies of the same patch, far enough apart not to overlap.
	frame, patch := synthFrame(600, 400, 100, 100, 50, 40)
	stride := frame.Stride
	for dy := 0; dy < 40; dy++ {
		for dx := 0; dx < 50; dx++ {
			srcO := (100+dy)*stride + (100+dx)*4
			dstO := (250+dy)*stride + (400+dx)*4
			copy(frame.Pix[dstO:dstO+4], frame.Pix[srcO:srcO+4])
		}
	}

	tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	results, err := m.FindAll(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.95})
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d matches, want exactly 2 (overlap suppression failed?)", len(results))
	}

	// Both expected positions must appear, in either order.
	found := map[stdimage.Point]bool{}
	for _, r := range results {
		found[r.Location] = true
	}
	for _, want := range []stdimage.Point{{X: 100, Y: 100}, {X: 400, Y: 250}} {
		if !found[want] {
			t.Errorf("missing expected match at %v; got %v", want, results)
		}
	}

	// Results must be ordered strongest first.
	for i := 1; i < len(results); i++ {
		if results[i-1].Similarity < results[i].Similarity {
			t.Errorf("results not sorted by similarity: %.4f before %.4f",
				results[i-1].Similarity, results[i].Similarity)
		}
	}
}

// TestMatcherRejectsOversizedTemplate covers the case that crashes OpenCV if
// unguarded: a template larger than the search region.
func TestMatcherRejectsOversizedTemplate(t *testing.T) {
	frame, _ := synthFrame(200, 150, 20, 20, 30, 30)
	_, bigPatch := synthFrame(400, 300, 0, 0, 400, 300)

	tpl, err := ggvimage.NewTemplateFromImage("big", bigPatch)
	if err != nil {
		t.Fatalf("NewTemplateFromImage: %v", err)
	}
	defer tpl.Close()

	m := ggvimage.NewMatcher()
	defer m.Close()

	_, err = m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.5})
	if err == nil {
		t.Error("expected an error when the template is larger than the frame")
	}
}

// TestMatcherReusableAcrossFrameSizes exercises the buffer-reuse path: the
// Matcher caches Mats sized to the frame, so a size change must reallocate
// rather than read stale or undersized memory.
func TestMatcherReusableAcrossFrameSizes(t *testing.T) {
	m := ggvimage.NewMatcher()
	defer m.Close()

	for _, dim := range [][2]int{{600, 400}, {320, 240}, {800, 600}, {320, 240}} {
		frame, patch := synthFrame(dim[0], dim[1], 50, 50, 40, 30)

		tpl, err := ggvimage.NewTemplateFromImage("patch", patch)
		if err != nil {
			t.Fatalf("NewTemplateFromImage at %dx%d: %v", dim[0], dim[1], err)
		}

		got, err := m.Find(frame, tpl, &ggvimage.MatchOptions{Threshold: 0.9})
		if err != nil {
			tpl.Close()
			t.Fatalf("Find at %dx%d: %v", dim[0], dim[1], err)
		}
		if got.Location.X != 50 || got.Location.Y != 50 {
			t.Errorf("at %dx%d: Location = %v, want (50,50)", dim[0], dim[1], got.Location)
		}
		tpl.Close()
	}
}

// TestRawFrameCloneIsIndependent covers the documented aliasing contract: Pix
// aliases the capturer's reused buffer, so anything kept beyond the current
// iteration must be cloned.
func TestRawFrameCloneIsIndependent(t *testing.T) {
	frame, _ := synthFrame(64, 48, 10, 10, 8, 8)

	clone := frame.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil")
	}

	original := clone.Pix[0]
	frame.Pix[0] = original + 40 // simulate the next Capture overwriting the buffer

	if clone.Pix[0] != original {
		t.Error("Clone shares the original's backing array")
	}
	if clone.Width != frame.Width || clone.Height != frame.Height || clone.Stride != frame.Stride {
		t.Error("Clone did not preserve frame geometry")
	}
}

// TestRawFrameToRGBAChannelOrder pins the BGRA-to-RGBA conversion. Getting this
// backwards produces images that look plausible but have red and blue swapped,
// which no size or emptiness check would catch.
func TestRawFrameToRGBAChannelOrder(t *testing.T) {
	// One pixel: B=10, G=20, R=30, A=255.
	frame := &capture.RawFrame{
		Pix:    []byte{10, 20, 30, 255},
		Width:  1,
		Height: 1,
		Stride: 4,
	}

	img := frame.ToRGBA()
	r, g, b, a := img.At(0, 0).RGBA()

	// RGBA() returns 16-bit values; 30<<8|30 == 0x1E1E for an 8-bit 30.
	if got, want := r>>8, uint32(30); got != want {
		t.Errorf("red = %d, want %d (B and R swapped?)", got, want)
	}
	if got, want := g>>8, uint32(20); got != want {
		t.Errorf("green = %d, want %d", got, want)
	}
	if got, want := b>>8, uint32(10); got != want {
		t.Errorf("blue = %d, want %d (B and R swapped?)", got, want)
	}
	if got, want := a>>8, uint32(255); got != want {
		t.Errorf("alpha = %d, want %d", got, want)
	}
}
