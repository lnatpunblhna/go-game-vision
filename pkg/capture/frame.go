package capture

import (
	"image"

	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// RawFrame is one captured frame, still in the platform's native pixel layout.
//
// Pix holds BGRA bytes with premultiplied alpha — the layout both macOS
// (ScreenCaptureKit) and Windows (GetDIBits) hand back natively, and the layout
// OpenCV expects for a 4-channel Mat. Keeping frames in this form is what lets
// the hot path skip a per-pixel conversion: turning a Retina window's 4.1M
// pixels into image.RGBA costs a 16MB allocation plus a full byte-swap pass
// every single frame, which is pure waste when the consumer is OpenCV.
//
// Pix aliases a buffer owned and reused by the FrameCapturer that produced it.
// It is overwritten by the next Capture call, so anything that needs to outlive
// the current iteration must call Clone first.
type RawFrame struct {
	Pix    []byte // BGRA, premultiplied alpha
	Width  int
	Height int
	Stride int // bytes per row; may exceed Width*4

	// Window is a snapshot of the source window's geometry at capture time.
	// Carried along so a match found in this frame can be mapped to click
	// coordinates without a second, possibly-inconsistent window query.
	Window WindowInfo
}

// Clone returns a RawFrame backed by its own copy of the pixels, safe to keep
// after the producing FrameCapturer moves on.
func (f *RawFrame) Clone() *RawFrame {
	if f == nil {
		return nil
	}
	dup := *f
	dup.Pix = make([]byte, len(f.Pix))
	copy(dup.Pix, f.Pix)
	return &dup
}

// Bounds returns the frame's pixel bounds.
func (f *RawFrame) Bounds() image.Rectangle {
	if f == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, f.Width, f.Height)
}

// ToRGBA converts the frame into a standard Go image.
//
// This is the slow, convenient path: it allocates and byte-swaps every pixel.
// Use it for saving screenshots or interop with image/*, not inside a capture
// loop — the whole point of RawFrame is to avoid this when the consumer is
// OpenCV, which reads BGRA directly.
func (f *RawFrame) ToRGBA() *image.RGBA {
	if f == nil || f.Width <= 0 || f.Height <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}

	img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		srcRow := f.Pix[y*f.Stride : y*f.Stride+f.Width*4]
		dstRow := img.Pix[y*img.Stride : y*img.Stride+f.Width*4]
		for x := 0; x < f.Width*4; x += 4 {
			dstRow[x+0] = srcRow[x+2] // R
			dstRow[x+1] = srcRow[x+1] // G
			dstRow[x+2] = srcRow[x+0] // B
			dstRow[x+3] = srcRow[x+3] // A
		}
	}
	return img
}

// FrameCapturer is a capture session bound to one window, built for repeated
// capture.
//
// The stateless ScreenCapture.CaptureWindowByPID re-resolves the process, the
// window and the display scale on every call; on macOS that includes
// enumerating shareable content, which alone costs tens of milliseconds. A
// session resolves all of it once and reuses its pixel buffer, so steady-state
// capture performs no allocation.
//
// Not safe for concurrent use: the reused buffer would tear. Use one session per
// goroutine, or serialise access.
type FrameCapturer interface {
	// Capture grabs the current window contents. The returned frame's Pix
	// aliases the session's buffer and is invalidated by the next Capture.
	Capture() (*RawFrame, error)

	// Window returns the cached window geometry, including ScaleFactor.
	Window() *WindowInfo

	// Refresh re-resolves the window geometry. Capture already tracks moves and
	// resizes, so this is only needed after the target window is recreated —
	// for example when a game switches between windowed and fullscreen.
	Refresh() error

	// Close releases platform resources held by the session.
	Close() error
}

// NewFrameCapturer creates a capture session for the given process's main
// window.
func NewFrameCapturer(pid uint32) (FrameCapturer, error) {
	platform.EnsureDPIAware()

	fc, err := newPlatformFrameCapturer(pid)
	if err != nil {
		return nil, utils.WrapError(err, "failed to create frame capturer")
	}
	return fc, nil
}
