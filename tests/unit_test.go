package tests

import (
	stdimage "image"
	"image/color"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// These tests exercise offline, deterministic logic only. They do NOT require a
// running process, a real window, screen-recording permissions, or a display,
// so they are safe to run in CI (they do require OpenCV for the gocv-backed
// image tests, which CI installs).

// --- helpers ---------------------------------------------------------------

// solidImage returns a w×h image filled with a single color.
func solidImage(w, h int, c color.Color) *stdimage.RGBA {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// imageWithPatch draws a filled rectangle of color c at (px,py) with size pw×ph
// on top of a white background of size w×h.
func imageWithPatch(w, h, px, py, pw, ph int, c color.Color) *stdimage.RGBA {
	img := solidImage(w, h, color.RGBA{255, 255, 255, 255})
	for y := py; y < py+ph && y < h; y++ {
		for x := px; x < px+pw && x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// patternTemplate returns a small image with internal structure (a blue square
// inside a red one). A textured, non-uniform template is required for template
// matching: TM_CCOEFF_NORMED normalizes by variance, so a solid-color template
// (zero variance) is degenerate and yields 0/NaN.
func patternTemplate(size int) *stdimage.RGBA {
	t := solidImage(size, size, color.RGBA{220, 20, 20, 255}) // red
	inset := size / 4
	for y := inset; y < size-inset; y++ {
		for x := inset; x < size-inset; x++ {
			t.Set(x, y, color.RGBA{20, 20, 220, 255}) // blue center
		}
	}
	return t
}

// blit copies src onto dst with its top-left at (px,py).
func blit(dst *stdimage.RGBA, src *stdimage.RGBA, px, py int) {
	b := src.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(px+x, py+y, src.At(x, y))
		}
	}
}

// --- utils.IsError / WrapError --------------------------------------------

func TestIsErrorMatchesWrappedSentinel(t *testing.T) {
	wrapped := utils.WrapError(utils.ErrProcessNotFound, "outer context")
	if wrapped == nil {
		t.Fatal("WrapError should not return nil for a non-nil error")
	}
	if !utils.IsError(wrapped, utils.ErrProcessNotFound) {
		t.Errorf("IsError should match a wrapped sentinel via errors.Is; got false for %v", wrapped)
	}
	if utils.IsError(wrapped, utils.ErrWindowNotFound) {
		t.Error("IsError should not match an unrelated sentinel")
	}
}

func TestWrapErrorNilReturnsNil(t *testing.T) {
	if err := utils.WrapError(nil, "context"); err != nil {
		t.Errorf("WrapError(nil, ...) should be nil, got %v", err)
	}
}

// --- ParseCompareMethod / GetMethodName -----------------------------------

func TestParseCompareMethodRoundTrip(t *testing.T) {
	cases := map[string]ggvimage.CompareMethod{
		"template":   ggvimage.TemplateMatching,
		"feature":    ggvimage.FeatureMatching,
		"histogram":  ggvimage.HistogramComparison,
		"similarity": ggvimage.StructuralSimilarity,
		"multiscale": ggvimage.MultiScaleTemplate,
	}
	for input, want := range cases {
		if got := ggvimage.ParseCompareMethod(input); got != want {
			t.Errorf("ParseCompareMethod(%q) = %v, want %v", input, got, want)
		}
	}
	// Unknown falls back to template matching.
	if got := ggvimage.ParseCompareMethod("nope"); got != ggvimage.TemplateMatching {
		t.Errorf("unknown method should fall back to TemplateMatching, got %v", got)
	}
	if name := ggvimage.GetMethodName(ggvimage.MultiScaleTemplate); name == "" || name == "Unknown" {
		t.Errorf("GetMethodName(MultiScaleTemplate) unexpected: %q", name)
	}
}

// --- coordinate conversion -------------------------------------------------

func TestToScreenCoordinatesAndBoundingBox(t *testing.T) {
	win := &capture.WindowInfo{
		Rect: stdimage.Rect(100, 200, 900, 800), // window at screen (100,200)
	}
	m := &ggvimage.MatchResult{
		Location:    stdimage.Point{X: 10, Y: 20},
		BoundingBox: stdimage.Rect(10, 20, 40, 60),
	}

	sc := m.ToScreenCoordinates(win)
	if sc.X != 110 || sc.Y != 220 {
		t.Errorf("ToScreenCoordinates = (%d,%d), want (110,220)", sc.X, sc.Y)
	}

	bb := m.ToScreenBoundingBox(win)
	if bb.Min.X != 110 || bb.Min.Y != 220 || bb.Max.X != 140 || bb.Max.Y != 260 {
		t.Errorf("ToScreenBoundingBox = %v, want (110,220)-(140,260)", bb)
	}
}

// --- enum String() ---------------------------------------------------------

func TestEnumStrings(t *testing.T) {
	if mouse.LeftButton.String() != "left" || mouse.RightButton.String() != "right" || mouse.MiddleButton.String() != "middle" {
		t.Error("MouseButton.String mismatch")
	}
	if capture.PNG.String() != "png" || capture.JPEG.String() != "jpeg" {
		t.Error("ImageFormat.String mismatch")
	}
	if opt := mouse.DefaultClickOptions(); opt.Button != mouse.LeftButton || opt.Delay != 50 {
		t.Errorf("DefaultClickOptions unexpected: %+v", opt)
	}
}

// --- template matching (gocv) ---------------------------------------------

func TestTemplateMatchingFindsPatch(t *testing.T) {
	template := patternTemplate(20)
	source := solidImage(120, 120, color.RGBA{255, 255, 255, 255})
	blit(source, template, 30, 40) // embed the exact template at (30,40)

	res, err := ggvimage.CompareImages(source, template, ggvimage.TemplateMatching)
	if err != nil {
		t.Fatalf("CompareImages error: %v", err)
	}
	if res.Similarity < 0.9 {
		t.Errorf("expected high similarity for exact patch, got %.4f", res.Similarity)
	}
	// Location should be at/near the patch top-left (30,40).
	if abs(res.Location.X-30) > 2 || abs(res.Location.Y-40) > 2 {
		t.Errorf("expected match near (30,40), got (%d,%d)", res.Location.X, res.Location.Y)
	}
}

func TestTemplateLargerThanSourceIsSafe(t *testing.T) {
	source := solidImage(20, 20, color.RGBA{0, 0, 0, 255})
	template := solidImage(50, 50, color.RGBA{0, 0, 0, 255})

	// Must not panic/crash; should report no match.
	res, err := ggvimage.CompareImages(source, template, ggvimage.TemplateMatching)
	if err != nil {
		t.Fatalf("CompareImages error: %v", err)
	}
	if res.Similarity != 0 {
		t.Errorf("template larger than source should yield 0 similarity, got %.4f", res.Similarity)
	}
}

// --- SSIM (gocv) -----------------------------------------------------------

func TestStructuralSimilarityIdenticalIsHigh(t *testing.T) {
	a := imageWithPatch(80, 80, 10, 10, 30, 30, color.RGBA{50, 120, 200, 255})
	b := imageWithPatch(80, 80, 10, 10, 30, 30, color.RGBA{50, 120, 200, 255})

	res, err := ggvimage.CompareImages(a, b, ggvimage.StructuralSimilarity)
	if err != nil {
		t.Fatalf("SSIM error: %v", err)
	}
	if res.Similarity < 0.99 {
		t.Errorf("SSIM of identical images should be ~1, got %.4f", res.Similarity)
	}
}

func TestStructuralSimilarityDifferentIsLower(t *testing.T) {
	a := solidImage(80, 80, color.RGBA{0, 0, 0, 255})
	b := imageWithPatch(80, 80, 0, 0, 40, 80, color.RGBA{255, 255, 255, 255}) // half white

	identical, _ := ggvimage.CompareImages(a, a, ggvimage.StructuralSimilarity)
	different, _ := ggvimage.CompareImages(a, b, ggvimage.StructuralSimilarity)
	if !(different.Similarity < identical.Similarity) {
		t.Errorf("SSIM should rank different < identical, got different=%.4f identical=%.4f",
			different.Similarity, identical.Similarity)
	}
}

// --- FindAndClick threshold (no real click on the no-match path) -----------

func TestFindAndClickBelowThresholdReturnsErrNoMatch(t *testing.T) {
	source := solidImage(120, 120, color.RGBA{0, 0, 0, 255})       // all black
	template := solidImage(20, 20, color.RGBA{255, 255, 255, 255}) // white patch, won't match

	win := &capture.WindowInfo{Rect: stdimage.Rect(0, 0, 120, 120)}
	// High threshold ensures the no-match branch (which returns before clicking).
	_, err := ggvimage.FindAndClickWithThreshold(source, template, win, ggvimage.TemplateMatching, 0.95, mouse.DefaultClickOptions())
	if err == nil {
		t.Fatal("expected ErrNoMatch error, got nil")
	}
	if !utils.IsError(err, ggvimage.ErrNoMatch) {
		t.Errorf("expected errors.Is(err, ErrNoMatch), got %v", err)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
