//go:build darwin

package capture

/*
	#include "window_query.h"
	#include "sck_capture.h"
*/
import "C"
import (
	"fmt"
	"image"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// darwinFrameCapturer is the macOS capture session.
//
// It resolves the window ID and display scale once, then reuses a single pixel
// buffer across frames. The expensive part of an SCK capture — enumerating
// shareable content — is cached in the C layer keyed by window ID, so steady
// state is one screenshot call plus a cheap bounds read.
type darwinFrameCapturer struct {
	pid      uint32
	windowID C.long
	info     WindowInfo
	buf      []byte
	closed   bool
}

func newPlatformFrameCapturer(pid uint32) (FrameCapturer, error) {
	fc := &darwinFrameCapturer{pid: pid}
	if err := fc.resolve(); err != nil {
		return nil, err
	}
	if !sckAvailable() {
		return nil, utils.WrapError(utils.ErrPlatformNotSupported,
			"FrameCapturer requires ScreenCaptureKit (macOS 14+); use CaptureWindowByPID for the slower fallback path")
	}
	return fc, nil
}

// resolve looks up the window ID, geometry and display scale for the session's
// PID.
func (d *darwinFrameCapturer) resolve() error {
	windowID := C.getWindowIDByPID(C.int(d.pid))
	if windowID == -1 {
		return utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("no window found for PID %d", d.pid))
	}
	if !bool(C.isWindowValid(windowID)) {
		return utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("window %d is invalid or closed", int64(windowID)))
	}

	d.windowID = windowID
	d.info = WindowInfo{
		Handle:      uintptr(windowID),
		PID:         d.pid,
		ScaleFactor: float64(C.getWindowScaleFactor(windowID)),
	}
	if !d.syncBounds() {
		return utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("could not read bounds of window %d", int64(windowID)))
	}
	return nil
}

// syncBounds refreshes the cached window rectangle. Called every frame: a window
// the user has dragged still captures correctly, but its origin has moved, and
// click coordinates are computed from that origin.
func (d *darwinFrameCapturer) syncBounds() bool {
	var x, y, w, h C.int
	if !bool(C.sckGetWindowBounds(d.windowID, &x, &y, &w, &h)) {
		return false
	}
	d.info.Rect = image.Rect(int(x), int(y), int(x)+int(w), int(y)+int(h))
	return true
}

func (d *darwinFrameCapturer) Capture() (*RawFrame, error) {
	if d.closed {
		return nil, utils.WrapError(utils.ErrCaptureFailure, "frame capturer is closed")
	}

	// Track window movement before capturing, so the geometry reported alongside
	// the frame matches the frame itself.
	if !d.syncBounds() {
		sckInvalidateWindowCache()
		return nil, utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("window %d disappeared", int64(d.windowID)))
	}

	scale := d.info.ScaleOrDefault()

	// Two attempts at most: the first may report -7 (buffer too small) while
	// filling in the geometry, which is exactly the information needed to size
	// the buffer. After that the buffer is reused with no allocation.
	for attempt := 0; attempt < 2; attempt++ {
		var w, h, stride C.int
		var dst *C.uchar
		if len(d.buf) > 0 {
			dst = (*C.uchar)(unsafe.Pointer(&d.buf[0]))
		}

		ret := C.sckCaptureWindowInto(d.windowID, C.double(scale),
			dst, C.int(len(d.buf)), &w, &h, &stride)

		switch {
		case ret == 0:
			return &RawFrame{
				Pix:    d.buf[:int(stride)*int(h)],
				Width:  int(w),
				Height: int(h),
				Stride: int(stride),
				Window: d.info,
			}, nil

		case ret == -7:
			need := int(stride) * int(h)
			if need <= 0 || need > maxFrameBytes {
				return nil, utils.WrapError(utils.ErrCaptureFailure,
					fmt.Sprintf("implausible frame size %dx%d (stride %d)", int(w), int(h), int(stride)))
			}
			if len(d.buf) >= need {
				// Should be unreachable: the buffer already covers the reported
				// need, yet the C layer said otherwise. Fail rather than loop.
				return nil, utils.WrapError(utils.ErrCaptureFailure,
					"capture buffer sizing disagreement")
			}
			d.buf = make([]byte, need)

		default:
			return nil, &sckCaptureError{code: int(ret)}
		}
	}

	return nil, utils.WrapError(utils.ErrCaptureFailure, "capture failed after buffer resize")
}

func (d *darwinFrameCapturer) Window() *WindowInfo {
	info := d.info
	return &info
}

func (d *darwinFrameCapturer) Refresh() error {
	sckInvalidateWindowCache()
	return d.resolve()
}

func (d *darwinFrameCapturer) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	d.buf = nil
	sckInvalidateWindowCache()
	return nil
}

// maxFrameBytes caps a single frame at 1GiB, which no real window can reach
// (an 8K Retina frame is ~530MB). Guards against a corrupt stride turning into
// a huge allocation.
const maxFrameBytes = 1 << 30
