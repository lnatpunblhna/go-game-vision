//go:build windows

package capture

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// windowsFrameCapturer is the Windows capture session.
//
// Beyond reusing the pixel buffer, this reuses the GDI objects: a memory DC and
// a compatible bitmap are created once and kept for the session's lifetime.
// Creating and destroying them per frame — as the stateless path does — is a
// large part of the per-frame cost, and leaking them is the classic way to
// exhaust the GDI object quota in a long-running loop.
type windowsFrameCapturer struct {
	pid  uint32
	hwnd uintptr
	info WindowInfo

	buf []byte

	// Cached GDI state. bmpW/bmpH track what the bitmap was created for, so it
	// is only rebuilt when the window is resized.
	screenDC  uintptr
	memDC     uintptr
	bitmap    uintptr
	oldBitmap uintptr
	bmpW      int
	bmpH      int

	// usePrintWindow selects PrintWindow (captures occluded windows) over BitBlt.
	usePrintWindow bool

	closed bool
}

func newPlatformFrameCapturer(pid uint32) (FrameCapturer, error) {
	fc := &windowsFrameCapturer{
		pid: pid,
		// Occluded windows are the norm for background automation, so default to
		// the API that can render them.
		usePrintWindow: true,
	}

	if err := fc.resolve(); err != nil {
		return nil, err
	}

	screenDC, _, err := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, utils.WrapError(err, "GetDC failed")
	}
	fc.screenDC = screenDC

	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		procReleaseDC.Call(0, screenDC)
		return nil, utils.WrapError(err, "CreateCompatibleDC failed")
	}
	fc.memDC = memDC

	return fc, nil
}

// resolve finds the target window for the session's PID.
func (w *windowsFrameCapturer) resolve() error {
	capturer := &WindowsCapture{}
	windows, err := capturer.GetWindowsByPID(w.pid)
	if err != nil {
		return utils.WrapError(err, "failed to enumerate windows")
	}
	if len(windows) == 0 {
		return utils.ErrWindowNotFound
	}

	// Prefer a visible window; fall back to the first one.
	target := windows[0]
	for _, win := range windows {
		if capturer.isWindowVisible(win.Handle) {
			target = win
			break
		}
	}

	w.hwnd = target.Handle
	w.info = target
	w.info.ScaleFactor = 1.0 // per-monitor DPI aware: rect and pixels agree
	return w.syncBounds()
}

// syncBounds refreshes the cached window rectangle. Called every frame so a
// window the user has moved still yields correct click coordinates.
func (w *windowsFrameCapturer) syncBounds() error {
	var rect RECT
	ret, _, err := procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return utils.WrapError(err, "GetWindowRect failed")
	}

	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid window size %dx%d (window minimised?)", width, height)
	}

	w.info.Rect = image.Rect(int(rect.Left), int(rect.Top), int(rect.Right), int(rect.Bottom))
	return nil
}

// ensureBitmap (re)creates the cached bitmap when the window size changes.
func (w *windowsFrameCapturer) ensureBitmap(width, height int) error {
	if w.bitmap != 0 && w.bmpW == width && w.bmpH == height {
		return nil
	}

	// Deselect and destroy the previous bitmap before replacing it; skipping this
	// is how a resize loop leaks GDI handles until the process hits its quota.
	w.releaseBitmap()

	bitmap, _, err := procCreateCompatibleBitmap.Call(w.screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return utils.WrapError(err, "CreateCompatibleBitmap failed")
	}

	oldBitmap, _, _ := procSelectObject.Call(w.memDC, bitmap)
	if oldBitmap == 0 {
		procDeleteObject.Call(bitmap)
		return utils.WrapError(nil, "SelectObject failed")
	}

	w.bitmap = bitmap
	w.oldBitmap = oldBitmap
	w.bmpW = width
	w.bmpH = height

	need := width * height * 4
	if len(w.buf) < need {
		w.buf = make([]byte, need)
	}
	return nil
}

func (w *windowsFrameCapturer) releaseBitmap() {
	if w.bitmap == 0 {
		return
	}
	if w.oldBitmap != 0 {
		procSelectObject.Call(w.memDC, w.oldBitmap)
		w.oldBitmap = 0
	}
	procDeleteObject.Call(w.bitmap)
	w.bitmap = 0
	w.bmpW, w.bmpH = 0, 0
}

func (w *windowsFrameCapturer) Capture() (*RawFrame, error) {
	if w.closed {
		return nil, utils.WrapError(utils.ErrCaptureFailure, "frame capturer is closed")
	}

	if err := w.syncBounds(); err != nil {
		return nil, err
	}

	width := w.info.Rect.Dx()
	height := w.info.Rect.Dy()
	if err := w.ensureBitmap(width, height); err != nil {
		return nil, err
	}

	if w.usePrintWindow {
		ret, _, _ := procPrintWindow.Call(w.hwnd, w.memDC, PW_RENDERFULLCONTENT)
		if ret == 0 {
			// Some windows (notably hardware-accelerated ones) refuse
			// PrintWindow. Fall back once and remember, so we stop paying for a
			// failing call on every subsequent frame.
			utils.Warn("PrintWindow failed for hwnd %d, falling back to BitBlt for this session", w.hwnd)
			w.usePrintWindow = false
		}
	}

	if !w.usePrintWindow {
		windowDC, _, err := procGetDC.Call(w.hwnd)
		if windowDC == 0 {
			return nil, utils.WrapError(err, "GetDC(hwnd) failed")
		}
		ret, _, _ := procBitBlt.Call(w.memDC, 0, 0, uintptr(width), uintptr(height),
			windowDC, 0, 0, SRCCOPY)
		procReleaseDC.Call(w.hwnd, windowDC)
		if ret == 0 {
			return nil, utils.WrapError(nil, "BitBlt failed")
		}
	}

	// GetDIBits wants the bitmap not selected into a DC.
	procSelectObject.Call(w.memDC, w.oldBitmap)

	var bi BITMAPINFO
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = int32(width)
	bi.BmiHeader.BiHeight = -int32(height) // negative = top-down rows
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = 0

	ret, _, _ := procGetDIBits.Call(
		w.screenDC,
		w.bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&w.buf[0])),
		uintptr(unsafe.Pointer(&bi)),
		DIB_RGB_COLORS,
	)

	// Reselect so the next frame can draw into it again.
	procSelectObject.Call(w.memDC, w.bitmap)

	if ret == 0 {
		return nil, utils.WrapError(nil, "GetDIBits failed")
	}

	stride := width * 4
	return &RawFrame{
		// GetDIBits with 32bpp already produces BGRA, which is exactly what
		// RawFrame promises — no per-pixel conversion needed.
		Pix:    w.buf[:stride*height],
		Width:  width,
		Height: height,
		Stride: stride,
		Window: w.info,
	}, nil
}

func (w *windowsFrameCapturer) Window() *WindowInfo {
	info := w.info
	return &info
}

func (w *windowsFrameCapturer) Refresh() error {
	return w.resolve()
}

func (w *windowsFrameCapturer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	w.releaseBitmap()
	if w.memDC != 0 {
		procDeleteDC.Call(w.memDC)
		w.memDC = 0
	}
	if w.screenDC != 0 {
		procReleaseDC.Call(0, w.screenDC)
		w.screenDC = 0
	}
	w.buf = nil
	return nil
}
