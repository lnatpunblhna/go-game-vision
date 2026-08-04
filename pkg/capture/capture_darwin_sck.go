//go:build darwin

package capture

/*
	#cgo CFLAGS: -x objective-c -fobjc-arc
	#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreMedia -framework CoreGraphics -framework Foundation
	#include <stdlib.h>
	#include "sck_capture.h"
*/
import "C"
import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// ScreenCaptureKit-backed window capture.
//
// This replaces the previous `screencapture -l<id>` subprocess for the hot path.
// The old approach forked a process, encoded a PNG, wrote it to /tmp and decoded
// it back — hundreds of milliseconds per frame, which makes any kind of
// automation loop impossible. SCK hands back a CGImage in memory instead.
//
// The subprocess path is kept in capture_darwin.go purely as a fallback for when
// SCK is unavailable or refuses the window (see CaptureWindowByPID).

// sckAvailable reports whether the running macOS provides the SCK screenshot
// entry point (macOS 14+). Probed once; the answer cannot change at runtime.
var (
	sckAvailableOnce sync.Once
	sckAvailableFlag bool
)

func sckAvailable() bool {
	sckAvailableOnce.Do(func() {
		sckAvailableFlag = bool(C.sckIsAvailable())
	})
	return sckAvailableFlag
}

// sckCaptureError describes why an SCK capture attempt failed. Callers use it to
// decide whether falling back to the subprocess path is worth trying.
type sckCaptureError struct {
	code int
}

func (e *sckCaptureError) Error() string {
	switch e.code {
	case -1:
		return "ScreenCaptureKit: window not found in shareable content (window closed, or Screen Recording permission not granted)"
	case -2:
		return "ScreenCaptureKit: window has zero size"
	case -3:
		return "ScreenCaptureKit: capture timed out or was refused (check Screen Recording permission)"
	case -4:
		return "ScreenCaptureKit: out of memory allocating pixel buffer"
	case -5:
		return "ScreenCaptureKit: failed to create bitmap context"
	case -6:
		return "ScreenCaptureKit: unavailable on this macOS version (requires macOS 14+)"
	default:
		return fmt.Sprintf("ScreenCaptureKit: capture failed with code %d", e.code)
	}
}

// captureWindowSCK captures the window with the given CoreGraphics window ID at
// the given scale factor, returning an RGBA image in physical pixels.
//
// scale must be the window's display scale factor: the configuration is sized in
// physical pixels, so passing 1.0 on a Retina display would yield a half-size,
// blurry capture that no longer matches WindowInfo.ScaleFactor.
func captureWindowSCK(windowID uintptr, scale float64) (*image.RGBA, error) {
	if !sckAvailable() {
		return nil, &sckCaptureError{code: -6}
	}
	if scale <= 0 {
		scale = 1.0
	}

	var out C.SCKImage
	ret := C.sckCaptureWindow(C.long(windowID), C.double(scale), &out)
	if ret != 0 {
		return nil, &sckCaptureError{code: int(ret)}
	}
	// The C side malloc'd this buffer; it is ours to free once copied.
	defer C.free(unsafe.Pointer(out.data))

	w, h := int(out.width), int(out.height)
	stride := int(out.bytesPerRow)
	if w <= 0 || h <= 0 {
		return nil, &sckCaptureError{code: -2}
	}

	// Reject implausible sizes before allocating, mirroring the guards in
	// pkg/image, so a corrupt width can't be turned into a huge allocation.
	if w > maxCaptureDimension || h > maxCaptureDimension {
		return nil, utils.WrapError(utils.ErrCaptureFailure,
			fmt.Sprintf("captured image too large: %dx%d", w, h))
	}

	src := unsafe.Slice((*byte)(unsafe.Pointer(out.data)), stride*h)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// SCK gives us BGRA (kCGBitmapByteOrder32Little + AlphaPremultipliedFirst);
	// image.RGBA wants R,G,B,A in that order and is likewise premultiplied, so
	// the conversion is a per-pixel byte swap of the B and R channels.
	for y := 0; y < h; y++ {
		srcRow := src[y*stride : y*stride+w*4]
		dstRow := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w*4; x += 4 {
			dstRow[x+0] = srcRow[x+2] // R <- R (stored third in BGRA)
			dstRow[x+1] = srcRow[x+1] // G
			dstRow[x+2] = srcRow[x+0] // B <- B (stored first in BGRA)
			dstRow[x+3] = srcRow[x+3] // A
		}
	}

	// The C buffer is referenced through an unsafe.Slice; make sure the deferred
	// free cannot be reordered before the copy finishes.
	runtime.KeepAlive(src)

	return img, nil
}

// maxCaptureDimension bounds a single dimension of a captured image. 32768
// matches the limit pkg/image already enforces on its inputs.
const maxCaptureDimension = 32768

// sckInvalidateWindowCache drops the cached SCShareableContent lookup, forcing
// the next capture to re-enumerate windows. Call this when a window is known to
// have been closed, moved between displays, or resized.
func sckInvalidateWindowCache() {
	C.sckInvalidateCache()
}
