package image

import (
	"fmt"
	"image"
	"os"
	"sort"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"gocv.io/x/gocv"
)

// The hot path.
//
// The convenience API in compare.go takes image.Image and rebuilds everything on
// every call: it decodes the template from disk, converts both images into fresh
// gocv.Mats, and scans a range of scales. That is fine for a one-off lookup and
// hopeless in a loop running at 20+ fps, where the conversions alone dominate.
//
// Matcher and Template are the loop-friendly counterparts:
//   - a Template is decoded into a Mat once, at startup, and reused;
//   - a Matcher owns reusable Mats so a steady-state match allocates nothing;
//   - RawFrame's native BGRA is handed to OpenCV directly, skipping the
//     per-pixel byte swap into image.RGBA entirely;
//   - a ROI restricts the search to the region that can actually contain the
//     target, which is usually the single biggest win available: searching a
//     200x200 button area instead of a 2146x1924 frame is ~100x less work.

// Template is a match target pre-decoded into an OpenCV matrix.
//
// Must be closed to release the underlying Mat; Go's GC does not manage OpenCV
// memory.
type Template struct {
	Name string

	mat  gocv.Mat // BGR, 3-channel
	w, h int
}

// LoadTemplate decodes an image file into a Template.
//
// Do this once at startup, not per frame: the previous helpers re-read and
// re-decoded the file on every call, which is wasted work when the template
// never changes.
func LoadTemplate(name, path string) (*Template, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, utils.WrapError(err, fmt.Sprintf("template %q not found at %s", name, path))
	}

	mat := gocv.IMRead(path, gocv.IMReadColor)
	if mat.Empty() {
		mat.Close()
		return nil, utils.WrapError(utils.ErrInvalidImage,
			fmt.Sprintf("template %q at %s could not be decoded", name, path))
	}

	return &Template{Name: name, mat: mat, w: mat.Cols(), h: mat.Rows()}, nil
}

// NewTemplateFromImage builds a Template from an in-memory image, for templates
// cropped at runtime rather than loaded from disk.
func NewTemplateFromImage(name string, img image.Image) (*Template, error) {
	mat, err := imageToMat(img)
	if err != nil {
		return nil, utils.WrapError(err, fmt.Sprintf("failed to convert template %q", name))
	}

	// imageToMat yields BGR already; guard anyway so a future change there cannot
	// silently produce a channel-count mismatch at match time.
	if mat.Channels() != 3 {
		bgr := gocv.NewMat()
		gocv.CvtColor(mat, &bgr, gocv.ColorBGRAToBGR)
		mat.Close()
		mat = bgr
	}

	return &Template{Name: name, mat: mat, w: mat.Cols(), h: mat.Rows()}, nil
}

// Size returns the template's dimensions in pixels.
func (t *Template) Size() image.Point {
	if t == nil {
		return image.Point{}
	}
	return image.Point{X: t.w, Y: t.h}
}

// Close releases the template's OpenCV matrix.
func (t *Template) Close() error {
	if t == nil || t.mat.Empty() {
		return nil
	}
	return t.mat.Close()
}

// MatchOptions configures a search.
type MatchOptions struct {
	// ROI restricts the search to this rectangle in frame pixel coordinates.
	// Zero value searches the whole frame. Coordinates in the returned
	// MatchResult are always frame-relative regardless of the ROI.
	ROI image.Rectangle

	// Threshold is the minimum similarity to accept, 0-1. Zero uses
	// DefaultMatchThreshold.
	Threshold float64

	// Scales lists template scale factors to try. Empty means {1.0} only.
	//
	// Leaving this empty is the right default for games: the window resolution is
	// fixed, so the DPI-normalised template matches at exactly one scale, and
	// scanning a range costs one full match per entry for nothing. Populate it
	// only when the target genuinely changes size.
	Scales []float64

	// MaxResults bounds FindAll's output. Zero means 10.
	MaxResults int
}

func (o *MatchOptions) threshold() float64 {
	if o == nil || o.Threshold <= 0 {
		return DefaultMatchThreshold
	}
	return o.Threshold
}

func (o *MatchOptions) scales() []float64 {
	if o == nil || len(o.Scales) == 0 {
		return []float64{1.0}
	}
	return o.Scales
}

func (o *MatchOptions) maxResults() int {
	if o == nil || o.MaxResults <= 0 {
		return 10
	}
	return o.MaxResults
}

// Matcher performs template searches against captured frames, reusing its
// intermediate buffers across calls.
//
// Not safe for concurrent use. One Matcher per capture loop.
type Matcher struct {
	bgra    gocv.Mat // frame pixels, 4-channel, reused
	bgr     gocv.Mat // search region converted to 3-channel, reused
	scaled  gocv.Mat // resized template, reused across scales
	result  gocv.Mat // matchTemplate output, reused
	noMask  gocv.Mat // empty Mat passed as matchTemplate's mask argument
	created bool
}

// NewMatcher creates a Matcher. Close it when done.
func NewMatcher() *Matcher {
	return &Matcher{
		bgra:    gocv.NewMat(),
		bgr:     gocv.NewMat(),
		scaled:  gocv.NewMat(),
		result:  gocv.NewMat(),
		noMask:  gocv.NewMat(),
		created: true,
	}
}

// Close releases the Matcher's OpenCV matrices.
func (m *Matcher) Close() error {
	if m == nil || !m.created {
		return nil
	}
	m.created = false
	m.bgra.Close()
	m.bgr.Close()
	m.scaled.Close()
	m.result.Close()
	m.noMask.Close()
	return nil
}

// prepareRegion loads the frame into reusable Mats and returns a BGR Mat of the
// search region, plus the region's origin in frame coordinates.
//
// The conversion to 3-channel happens *after* cropping to the ROI, so a small
// ROI also shrinks the conversion cost rather than only the match cost.
func (m *Matcher) prepareRegion(frame *capture.RawFrame, roi image.Rectangle) (gocv.Mat, image.Point, error) {
	var zero gocv.Mat

	if frame == nil || frame.Width <= 0 || frame.Height <= 0 {
		return zero, image.Point{}, utils.WrapError(utils.ErrInvalidImage, "empty frame")
	}
	expected := frame.Stride * frame.Height
	if len(frame.Pix) < expected {
		return zero, image.Point{}, utils.WrapError(utils.ErrInvalidImage,
			fmt.Sprintf("frame buffer too small: have %d bytes, need %d", len(frame.Pix), expected))
	}

	// (Re)allocate the BGRA holder only when the frame size changes.
	if m.bgra.Empty() || m.bgra.Rows() != frame.Height || m.bgra.Cols() != frame.Width {
		m.bgra.Close()
		m.bgra = gocv.NewMatWithSize(frame.Height, frame.Width, gocv.MatTypeCV8UC4)
	}

	dst, err := m.bgra.DataPtrUint8()
	if err != nil {
		return zero, image.Point{}, utils.WrapError(err, "failed to access matrix data")
	}

	// Copy row by row: the capture buffer's stride may include padding that the
	// Mat's does not.
	rowBytes := frame.Width * 4
	matStride := len(dst) / frame.Height
	for y := 0; y < frame.Height; y++ {
		copy(dst[y*matStride:y*matStride+rowBytes], frame.Pix[y*frame.Stride:y*frame.Stride+rowBytes])
	}

	// Clamp the ROI to the frame; an out-of-range Region would panic in OpenCV.
	bounds := image.Rect(0, 0, frame.Width, frame.Height)
	region := bounds
	if !roi.Empty() {
		region = roi.Intersect(bounds)
		if region.Empty() {
			return zero, image.Point{}, utils.WrapError(utils.ErrInvalidImage,
				fmt.Sprintf("ROI %v does not intersect frame %v", roi, bounds))
		}
	}

	src := m.bgra
	var cropped gocv.Mat
	if region != bounds {
		cropped = m.bgra.Region(region)
		src = cropped
	}

	gocv.CvtColor(src, &m.bgr, gocv.ColorBGRAToBGR)
	if region != bounds {
		cropped.Close() // Region returns a view; closing it does not free m.bgra
	}

	return m.bgr, region.Min, nil
}

// templateAtScale returns the template resized by the given factor. Returns the
// original Mat at scale 1.0 to avoid a pointless copy.
func (m *Matcher) templateAtScale(tpl *Template, scale float64) (gocv.Mat, error) {
	if scale == 1.0 {
		return tpl.mat, nil
	}

	w := int(float64(tpl.w)*scale + 0.5)
	h := int(float64(tpl.h)*scale + 0.5)
	if w < 1 || h < 1 {
		return gocv.Mat{}, fmt.Errorf("scale %.2f reduces template %q to %dx%d", scale, tpl.Name, w, h)
	}

	gocv.Resize(tpl.mat, &m.scaled, image.Point{X: w, Y: h}, 0, 0, gocv.InterpolationLinear)
	return m.scaled, nil
}

// Find locates the single best occurrence of tpl in frame.
//
// Returns ErrNoMatch when nothing reaches the threshold. Coordinates in the
// result are frame-relative pixels even when a ROI was used; pair with
// frame.Window and MatchResult.ToScreenCenter to get a click point.
func (m *Matcher) Find(frame *capture.RawFrame, tpl *Template, opts *MatchOptions) (*MatchResult, error) {
	if tpl == nil || tpl.mat.Empty() {
		return nil, utils.WrapError(utils.ErrInvalidImage, "nil or empty template")
	}

	region, origin, err := m.prepareRegion(frame, opts.ROI)
	if err != nil {
		return nil, err
	}

	var best *MatchResult
	for _, scale := range opts.scales() {
		tplMat, err := m.templateAtScale(tpl, scale)
		if err != nil {
			utils.Debug("skipping scale %.2f for %q: %v", scale, tpl.Name, err)
			continue
		}

		// matchTemplate requires the template to fit inside the search region;
		// violating this throws inside OpenCV rather than returning an error.
		if tplMat.Cols() > region.Cols() || tplMat.Rows() > region.Rows() {
			utils.Debug("template %q at scale %.2f (%dx%d) exceeds search region (%dx%d)",
				tpl.Name, scale, tplMat.Cols(), tplMat.Rows(), region.Cols(), region.Rows())
			continue
		}

		gocv.MatchTemplate(region, tplMat, &m.result, gocv.TmCcoeffNormed, m.noMask)
		_, maxVal, _, maxLoc := gocv.MinMaxLoc(m.result)

		if best != nil && float64(maxVal) <= best.Similarity {
			continue
		}

		// Translate from region-local back to frame coordinates.
		loc := image.Point{X: maxLoc.X + origin.X, Y: maxLoc.Y + origin.Y}
		best = &MatchResult{
			Similarity: float64(maxVal),
			Confidence: float64(maxVal),
			Location:   loc,
			Method:     TemplateMatching,
			Scale:      scale,
			BoundingBox: image.Rectangle{
				Min: loc,
				Max: image.Point{X: loc.X + tplMat.Cols(), Y: loc.Y + tplMat.Rows()},
			},
		}
	}

	if best == nil {
		return nil, utils.WrapError(ErrNoMatch,
			fmt.Sprintf("template %q could not be matched at any scale", tpl.Name))
	}
	if best.Similarity < opts.threshold() {
		return nil, utils.WrapError(ErrNoMatch,
			fmt.Sprintf("best match for %q was %.3f, below threshold %.3f",
				tpl.Name, best.Similarity, opts.threshold()))
	}
	return best, nil
}

// FindAll locates every occurrence of tpl reaching the threshold, strongest
// first, suppressing overlapping detections.
func (m *Matcher) FindAll(frame *capture.RawFrame, tpl *Template, opts *MatchOptions) ([]*MatchResult, error) {
	if tpl == nil || tpl.mat.Empty() {
		return nil, utils.WrapError(utils.ErrInvalidImage, "nil or empty template")
	}

	region, origin, err := m.prepareRegion(frame, opts.ROI)
	if err != nil {
		return nil, err
	}

	threshold := opts.threshold()
	var candidates []*MatchResult

	for _, scale := range opts.scales() {
		tplMat, err := m.templateAtScale(tpl, scale)
		if err != nil {
			continue
		}
		if tplMat.Cols() > region.Cols() || tplMat.Rows() > region.Rows() {
			continue
		}

		gocv.MatchTemplate(region, tplMat, &m.result, gocv.TmCcoeffNormed, m.noMask)

		data, err := m.result.DataPtrFloat32()
		if err != nil {
			return nil, utils.WrapError(err, "failed to read match scores")
		}

		rows, cols := m.result.Rows(), m.result.Cols()
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				score := float64(data[y*cols+x])
				if score < threshold {
					continue
				}
				loc := image.Point{X: x + origin.X, Y: y + origin.Y}
				candidates = append(candidates, &MatchResult{
					Similarity: score,
					Confidence: score,
					Location:   loc,
					Method:     TemplateMatching,
					Scale:      scale,
					BoundingBox: image.Rectangle{
						Min: loc,
						Max: image.Point{X: loc.X + tplMat.Cols(), Y: loc.Y + tplMat.Rows()},
					},
				})
			}
		}
	}

	if len(candidates) == 0 {
		return nil, utils.WrapError(ErrNoMatch,
			fmt.Sprintf("no occurrence of %q reached threshold %.3f", tpl.Name, threshold))
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Similarity > candidates[j].Similarity
	})

	// Suppress overlaps: a strong match produces a cluster of high scores around
	// its true position, which would otherwise be reported as many separate hits.
	var kept []*MatchResult
	limit := opts.maxResults()
	for _, c := range candidates {
		overlaps := false
		for _, k := range kept {
			if c.BoundingBox.Intersect(k.BoundingBox).Empty() {
				continue
			}
			overlaps = true
			break
		}
		if overlaps {
			continue
		}
		kept = append(kept, c)
		if len(kept) >= limit {
			break
		}
	}

	return kept, nil
}
