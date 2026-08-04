package automation

import (
	"fmt"
	stdimage "image"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// frameDiffSampleStep is the sampling stride for frame comparison.
//
// Comparing every pixel of a Retina frame means touching 16MB twice per poll,
// which is a lot of work to answer a yes/no question. Sampling every 4th pixel in
// each direction looks at 1/16th of the data and still catches any change large
// enough to matter for "has the screen settled" — a change too small to hit a
// 1-in-16 sample is also too small to be a state transition.
const frameDiffSampleStep = 4

// FrameDifference returns the fraction of sampled pixels that differ between two
// frames, in the range 0-1.
//
// channelDelta is how much one channel may differ before the pixel counts as
// changed; it absorbs compression and dithering noise, which would otherwise make
// a static screen register as perpetually changing.
//
// Only the B, G and R channels are compared. Alpha is skipped because window
// captures carry alpha that varies with compositing rather than with content, and
// letting it vote produces spurious differences.
func FrameDifference(a, b *capture.RawFrame, roi stdimage.Rectangle, channelDelta int) (float64, error) {
	if a == nil || b == nil {
		return 0, utils.WrapError(utils.ErrInvalidImage, "nil frame in comparison")
	}
	if a.Width != b.Width || a.Height != b.Height {
		// A resize is a change by definition, and there is no meaningful
		// per-pixel comparison between different geometries.
		return 1.0, nil
	}
	if a.Width <= 0 || a.Height <= 0 {
		return 0, utils.WrapError(utils.ErrInvalidImage, "empty frame in comparison")
	}
	if channelDelta < 0 {
		channelDelta = 0
	}

	region := stdimage.Rect(0, 0, a.Width, a.Height)
	if !roi.Empty() {
		region = roi.Intersect(region)
		if region.Empty() {
			return 0, utils.WrapError(utils.ErrInvalidParameter,
				fmt.Sprintf("ROI %v does not intersect frame %dx%d", roi, a.Width, a.Height))
		}
	}

	needA := a.Stride * a.Height
	needB := b.Stride * b.Height
	if len(a.Pix) < needA || len(b.Pix) < needB {
		return 0, utils.WrapError(utils.ErrInvalidImage, "frame buffer shorter than its geometry")
	}

	var sampled, changed int
	for y := region.Min.Y; y < region.Max.Y; y += frameDiffSampleStep {
		rowA := a.Pix[y*a.Stride:]
		rowB := b.Pix[y*b.Stride:]

		for x := region.Min.X; x < region.Max.X; x += frameDiffSampleStep {
			o := x * 4
			sampled++

			// Unrolled over B, G, R; alpha (index 3) intentionally ignored.
			if absDiff(rowA[o+0], rowB[o+0]) > channelDelta ||
				absDiff(rowA[o+1], rowB[o+1]) > channelDelta ||
				absDiff(rowA[o+2], rowB[o+2]) > channelDelta {
				changed++
			}
		}
	}

	if sampled == 0 {
		return 0, nil
	}
	return float64(changed) / float64(sampled), nil
}

func absDiff(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
