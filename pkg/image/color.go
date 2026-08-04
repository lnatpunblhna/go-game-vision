package image

import (
	"fmt"
	"image"
	"sort"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"gocv.io/x/gocv"
)

// Colour-threshold detection, for the game UI that template matching handles
// badly.
//
// A health bar is the canonical example. Its width encodes the value you want to
// read, so matching it against a template would need one template per health
// level — and the bar is often a gradient, semi-transparent, or animated, none of
// which template matching tolerates. Thresholding in HSV reads the value directly
// and costs a fraction of a full match. The same applies to mana bars, cooldown
// sweeps, buff highlights, and minimap blips.
//
// HSV rather than RGB because HSV separates "what colour" (hue) from "how bright"
// (value). A red bar in shadow and the same bar in sunlight share a hue but have
// very different RGB triples, so an RGB threshold that catches both also catches
// half the screen.

// ColorRange is an HSV threshold band.
//
// Channel ranges follow OpenCV's 8-bit convention, which is *not* the 0-360
// degree hue most colour pickers show: hue is 0-179 (degrees halved so it fits in
// a byte), saturation and value are 0-255. Halve any hue you read elsewhere.
type ColorRange struct {
	Name string

	// HMin/HMax bound hue, 0-179.
	//
	// HMin > HMax means the band wraps past 179 back through 0, which red
	// requires: red sits at both ends of the hue circle (roughly 170-179 and
	// 0-10), so it cannot be expressed as a single ascending interval. Detect is
	// aware of this and thresholds twice.
	HMin, HMax uint8

	// SMin/SMax bound saturation, 0-255. Raising SMin is the usual way to reject
	// washed-out greys that technically carry the target hue.
	SMin, SMax uint8

	// VMin/VMax bound value (brightness), 0-255. Raising VMin rejects near-black
	// pixels, whose hue is meaningless noise.
	VMin, VMax uint8
}

// Wraps reports whether the hue band crosses the 179/0 boundary.
func (r ColorRange) Wraps() bool { return r.HMin > r.HMax }

// String describes the range for logs.
func (r ColorRange) String() string {
	return fmt.Sprintf("%s(H %d-%d, S %d-%d, V %d-%d)",
		r.Name, r.HMin, r.HMax, r.SMin, r.SMax, r.VMin, r.VMax)
}

// Preset ranges covering the colours game UI most often uses.
//
// Treat these as starting points, not truths: every game picks its own palette,
// and the reliable way to set a range is to sample the actual pixels with
// SampleHSV and widen from there.
var (
	// RangeRed wraps the hue boundary; see ColorRange.HMin.
	RangeRed    = ColorRange{Name: "red", HMin: 170, HMax: 10, SMin: 90, SMax: 255, VMin: 60, VMax: 255}
	RangeGreen  = ColorRange{Name: "green", HMin: 40, HMax: 85, SMin: 80, SMax: 255, VMin: 50, VMax: 255}
	RangeBlue   = ColorRange{Name: "blue", HMin: 95, HMax: 130, SMin: 80, SMax: 255, VMin: 50, VMax: 255}
	RangeYellow = ColorRange{Name: "yellow", HMin: 20, HMax: 35, SMin: 90, SMax: 255, VMin: 90, VMax: 255}
	RangeCyan   = ColorRange{Name: "cyan", HMin: 85, HMax: 95, SMin: 80, SMax: 255, VMin: 60, VMax: 255}
	RangeOrange = ColorRange{Name: "orange", HMin: 11, HMax: 20, SMin: 110, SMax: 255, VMin: 90, VMax: 255}
	RangePurple = ColorRange{Name: "purple", HMin: 130, HMax: 160, SMin: 70, SMax: 255, VMin: 50, VMax: 255}
	// RangeWhite ignores hue entirely: white is defined by low saturation and
	// high value, and its hue is whatever noise happens to be there.
	RangeWhite = ColorRange{Name: "white", HMin: 0, HMax: 179, SMin: 0, SMax: 40, VMin: 200, VMax: 255}
	RangeBlack = ColorRange{Name: "black", HMin: 0, HMax: 179, SMin: 0, SMax: 255, VMin: 0, VMax: 45}
)

// Orientation is the direction a bar fills.
type Orientation int

const (
	// LeftToRight is the usual health bar: empties towards the right.
	LeftToRight Orientation = iota
	RightToLeft
	BottomToTop
	TopToBottom
)

// String returns a readable orientation name.
func (o Orientation) String() string {
	switch o {
	case LeftToRight:
		return "left-to-right"
	case RightToLeft:
		return "right-to-left"
	case BottomToTop:
		return "bottom-to-top"
	case TopToBottom:
		return "top-to-bottom"
	default:
		return "unknown"
	}
}

// Blob is a connected region of matching colour.
type Blob struct {
	BoundingBox image.Rectangle // in frame pixel coordinates
	Area        int             // matching pixels inside the contour
	Center      image.Point     // centroid, in frame pixel coordinates
}

// ColorDetector runs colour thresholds against frames, reusing its matrices.
//
// Not safe for concurrent use. Close it when finished.
type ColorDetector struct {
	bgra  gocv.Mat
	bgr   gocv.Mat
	hsv   gocv.Mat
	mask  gocv.Mat
	mask2 gocv.Mat // second band, for wrapping hue ranges
	col   gocv.Mat // per-column/row reduction for bar measurement

	created bool
}

// NewColorDetector creates a detector. Close it when done.
func NewColorDetector() *ColorDetector {
	return &ColorDetector{
		bgra:    gocv.NewMat(),
		bgr:     gocv.NewMat(),
		hsv:     gocv.NewMat(),
		mask:    gocv.NewMat(),
		mask2:   gocv.NewMat(),
		col:     gocv.NewMat(),
		created: true,
	}
}

// Close releases the detector's matrices.
func (d *ColorDetector) Close() error {
	if d == nil || !d.created {
		return nil
	}
	d.created = false
	d.bgra.Close()
	d.bgr.Close()
	d.hsv.Close()
	d.mask.Close()
	d.mask2.Close()
	d.col.Close()
	return nil
}

// buildMask converts the region of interest to HSV and thresholds it, leaving the
// binary result in d.mask. Returns the region's origin in frame coordinates.
//
// Cropping happens before the colour conversion so a small ROI also shrinks the
// conversion cost, not just the threshold.
func (d *ColorDetector) buildMask(frame *capture.RawFrame, roi image.Rectangle, r ColorRange) (image.Point, error) {
	if frame == nil || frame.Width <= 0 || frame.Height <= 0 {
		return image.Point{}, utils.WrapError(utils.ErrInvalidImage, "empty frame")
	}
	if need := frame.Stride * frame.Height; len(frame.Pix) < need {
		return image.Point{}, utils.WrapError(utils.ErrInvalidImage,
			fmt.Sprintf("frame buffer too small: have %d bytes, need %d", len(frame.Pix), need))
	}

	if d.bgra.Empty() || d.bgra.Rows() != frame.Height || d.bgra.Cols() != frame.Width {
		d.bgra.Close()
		d.bgra = gocv.NewMatWithSize(frame.Height, frame.Width, gocv.MatTypeCV8UC4)
	}

	dst, err := d.bgra.DataPtrUint8()
	if err != nil {
		return image.Point{}, utils.WrapError(err, "failed to access matrix data")
	}
	rowBytes := frame.Width * 4
	matStride := len(dst) / frame.Height
	for y := 0; y < frame.Height; y++ {
		copy(dst[y*matStride:y*matStride+rowBytes], frame.Pix[y*frame.Stride:y*frame.Stride+rowBytes])
	}

	bounds := image.Rect(0, 0, frame.Width, frame.Height)
	region := bounds
	if !roi.Empty() {
		region = roi.Intersect(bounds)
		if region.Empty() {
			return image.Point{}, utils.WrapError(utils.ErrInvalidParameter,
				fmt.Sprintf("ROI %v does not intersect frame %v", roi, bounds))
		}
	}

	src := d.bgra
	var cropped gocv.Mat
	if region != bounds {
		cropped = d.bgra.Region(region)
		src = cropped
	}
	gocv.CvtColor(src, &d.bgr, gocv.ColorBGRAToBGR)
	if region != bounds {
		cropped.Close()
	}

	gocv.CvtColor(d.bgr, &d.hsv, gocv.ColorBGRToHSV)

	if r.Wraps() {
		// Two bands, unioned: HMin..179 and 0..HMax.
		lower1 := gocv.NewScalar(float64(r.HMin), float64(r.SMin), float64(r.VMin), 0)
		upper1 := gocv.NewScalar(179, float64(r.SMax), float64(r.VMax), 0)
		gocv.InRangeWithScalar(d.hsv, lower1, upper1, &d.mask)

		lower2 := gocv.NewScalar(0, float64(r.SMin), float64(r.VMin), 0)
		upper2 := gocv.NewScalar(float64(r.HMax), float64(r.SMax), float64(r.VMax), 0)
		gocv.InRangeWithScalar(d.hsv, lower2, upper2, &d.mask2)

		gocv.BitwiseOr(d.mask, d.mask2, &d.mask)
	} else {
		lower := gocv.NewScalar(float64(r.HMin), float64(r.SMin), float64(r.VMin), 0)
		upper := gocv.NewScalar(float64(r.HMax), float64(r.SMax), float64(r.VMax), 0)
		gocv.InRangeWithScalar(d.hsv, lower, upper, &d.mask)
	}

	return region.Min, nil
}

// Ratio returns the fraction of pixels in the region matching the colour, 0-1.
//
// The cheapest useful colour query: good for "is this button highlighted",
// "is the buff icon lit", "is the screen mostly red because we are taking damage".
func (d *ColorDetector) Ratio(frame *capture.RawFrame, roi image.Rectangle, r ColorRange) (float64, error) {
	if _, err := d.buildMask(frame, roi, r); err != nil {
		return 0, err
	}

	total := d.mask.Rows() * d.mask.Cols()
	if total == 0 {
		return 0, nil
	}
	return float64(gocv.CountNonZero(d.mask)) / float64(total), nil
}

// BarLevelOptions tunes bar measurement.
type BarLevelOptions struct {
	// Orientation is the direction the bar fills. Default LeftToRight.
	Orientation Orientation

	// FillFraction is how much of a line (column for horizontal bars, row for
	// vertical) must match before that line counts as filled. Zero means
	// DefaultBarFillFraction.
	//
	// This is what makes the measurement robust against a bar's border, gloss
	// highlight or rounded ends: those leave part of every line unmatched, and
	// demanding a full line would read a full bar as empty.
	FillFraction float64
}

// DefaultBarFillFraction is the per-line match fraction BarLevel requires.
const DefaultBarFillFraction = 0.35

func (o *BarLevelOptions) orientation() Orientation {
	if o == nil {
		return LeftToRight
	}
	return o.Orientation
}

func (o *BarLevelOptions) fillFraction() float64 {
	if o == nil || o.FillFraction <= 0 {
		return DefaultBarFillFraction
	}
	return o.FillFraction
}

// BarLevel measures how full a bar is, returning 0-1.
//
// Deliberately not the same thing as Ratio. A bar has a border, often a gradient
// and sometimes a gloss overlay, so the fraction of matching *pixels* understates
// the fill. This instead asks, line by line along the fill direction, "is this
// line part of the filled section", and reports how far the filled run extends —
// which is the quantity a health bar actually encodes.
//
// Measures the run starting from the filled end, so a partially transparent empty
// section cannot inflate the reading.
func (d *ColorDetector) BarLevel(frame *capture.RawFrame, roi image.Rectangle, r ColorRange, opts *BarLevelOptions) (float64, error) {
	if _, err := d.buildMask(frame, roi, r); err != nil {
		return 0, err
	}

	rows, cols := d.mask.Rows(), d.mask.Cols()
	if rows == 0 || cols == 0 {
		return 0, nil
	}

	orientation := opts.orientation()
	horizontal := orientation == LeftToRight || orientation == RightToLeft

	// Reduce to a per-line sum of mask values. dim 0 collapses rows, yielding one
	// value per column; dim 1 collapses columns, one per row.
	//
	// CV32F, not the 8-bit mask type: a sum of 255s overflows a byte after a
	// single pixel. Float rather than int because gocv exposes DataPtrFloat32 but
	// has no DataPtrInt32.
	var dim int
	var lineCount, lineLength int
	if horizontal {
		dim = 0
		lineCount, lineLength = cols, rows
	} else {
		dim = 1
		lineCount, lineLength = rows, cols
	}

	gocv.Reduce(d.mask, &d.col, dim, gocv.ReduceSum, gocv.MatTypeCV32F)

	sums, err := d.col.DataPtrFloat32()
	if err != nil {
		return 0, utils.WrapError(err, "failed to read bar line sums")
	}
	if len(sums) < lineCount {
		return 0, utils.WrapError(utils.ErrImageProcessing,
			fmt.Sprintf("reduction produced %d values, expected %d", len(sums), lineCount))
	}

	// A line counts as filled when enough of it matched. Mask values are 0 or 255,
	// so divide the sum by 255 to recover a pixel count.
	required := float64(lineLength) * opts.fillFraction()
	if required < 1 {
		required = 1
	}

	matchedPixels := func(i int) float64 { return float64(sums[i]) / 255.0 }

	filled := 0
	switch orientation {
	case LeftToRight, TopToBottom:
		// Filled section starts at index 0.
		for i := 0; i < lineCount; i++ {
			if matchedPixels(i) < required {
				break
			}
			filled++
		}
	case RightToLeft, BottomToTop:
		// Filled section starts at the far end.
		for i := lineCount - 1; i >= 0; i-- {
			if matchedPixels(i) < required {
				break
			}
			filled++
		}
	}

	return float64(filled) / float64(lineCount), nil
}

// FindBlobs locates connected regions of the colour, largest first.
//
// Use for minimap blips, dropped loot markers, enemy nameplates — anything
// identified by colour and position rather than by an exact bitmap.
//
// minArea discards specks; set it above the noise floor for the game, otherwise
// compression artefacts register as blobs.
func (d *ColorDetector) FindBlobs(frame *capture.RawFrame, roi image.Rectangle, r ColorRange, minArea int) ([]Blob, error) {
	origin, err := d.buildMask(frame, roi, r)
	if err != nil {
		return nil, err
	}
	if minArea < 1 {
		minArea = 1
	}

	contours := gocv.FindContours(d.mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	blobs := make([]Blob, 0, contours.Size())
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := int(gocv.ContourArea(c))
		if area < minArea {
			continue
		}

		rect := gocv.BoundingRect(c)
		// Translate from ROI-local to frame coordinates, so callers can feed the
		// result straight into a click without knowing a ROI was involved.
		box := image.Rect(
			rect.Min.X+origin.X, rect.Min.Y+origin.Y,
			rect.Max.X+origin.X, rect.Max.Y+origin.Y,
		)
		blobs = append(blobs, Blob{
			BoundingBox: box,
			Area:        area,
			Center: image.Point{
				X: (box.Min.X + box.Max.X) / 2,
				Y: (box.Min.Y + box.Max.Y) / 2,
			},
		})
	}

	sort.Slice(blobs, func(i, j int) bool { return blobs[i].Area > blobs[j].Area })
	return blobs, nil
}

// SampleHSV reports the HSV value at one frame pixel.
//
// The practical way to build a ColorRange: sample the pixels you care about in the
// real game, then widen the band around what you measure. Guessing from a colour
// picker's RGB and converting by hand is how ranges end up subtly wrong — not
// least because OpenCV's hue is halved.
func (d *ColorDetector) SampleHSV(frame *capture.RawFrame, x, y int) (h, s, v uint8, err error) {
	if frame == nil {
		return 0, 0, 0, utils.WrapError(utils.ErrInvalidImage, "nil frame")
	}
	if x < 0 || y < 0 || x >= frame.Width || y >= frame.Height {
		return 0, 0, 0, utils.WrapError(utils.ErrInvalidParameter,
			fmt.Sprintf("point (%d,%d) is outside the frame %dx%d", x, y, frame.Width, frame.Height))
	}

	// Threshold a 1x1 region purely to reuse the BGRA->BGR->HSV pipeline.
	if _, err := d.buildMask(frame, image.Rect(x, y, x+1, y+1), RangeWhite); err != nil {
		return 0, 0, 0, err
	}

	px, err := d.hsv.DataPtrUint8()
	if err != nil {
		return 0, 0, 0, utils.WrapError(err, "failed to read HSV data")
	}
	if len(px) < 3 {
		return 0, 0, 0, utils.WrapError(utils.ErrImageProcessing, "HSV sample was empty")
	}
	return px[0], px[1], px[2], nil
}
