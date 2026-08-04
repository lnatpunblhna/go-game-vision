//go:build darwin

package capture

/*
	#include <stdlib.h>
	#include "window_query.h"
*/
import "C"
import (
	"fmt"
	"image"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// titleBufferSize matches the buffer size in window_query.m. Titles longer than
// this are truncated, which is fine for matching but means an exact-title query
// against a very long title will not match.
const titleBufferSize = 256

// findWindowsPlatform enumerates macOS windows and applies the query.
func findWindowsPlatform(query *WindowQuery) ([]FoundWindow, error) {
	count := int(C.enumerateWindows(C.bool(query.IncludeOffscreen)))
	if count < 0 {
		return nil, utils.WrapError(utils.ErrCaptureFailure, "failed to enumerate windows")
	}

	titleBuf := (*C.char)(C.malloc(titleBufferSize))
	defer C.free(unsafe.Pointer(titleBuf))
	ownerBuf := (*C.char)(C.malloc(titleBufferSize))
	defer C.free(unsafe.Pointer(ownerBuf))

	var found []FoundWindow
	for i := 0; i < count; i++ {
		var windowID C.long
		var pid, layer, x, y, w, h C.int

		if !bool(C.windowSnapshotAt(C.int(i),
			&windowID, &pid, &layer, &x, &y, &w, &h,
			titleBuf, titleBufferSize, ownerBuf, titleBufferSize)) {
			continue
		}

		info := WindowInfo{
			Handle: uintptr(windowID),
			Title:  C.GoString(titleBuf),
			PID:    uint32(pid),
			Rect:   image.Rect(int(x), int(y), int(x)+int(w), int(y)+int(h)),
		}
		owner := C.GoString(ownerBuf)

		if !query.matches(&info, owner, int(layer)) {
			continue
		}

		// Only resolve the scale factor for windows that survive filtering: it
		// costs another window-server round trip per call, which is wasted on the
		// hundred-odd windows a normal desktop has.
		info.ScaleFactor = float64(C.getWindowScaleFactor(windowID))

		found = append(found, FoundWindow{
			WindowInfo: info,
			OwnerName:  owner,
			Layer:      int(layer),
		})
	}

	return found, nil
}

// newPlatformFrameCapturerForWindow builds a session bound to a specific window
// ID rather than resolving one from a PID.
func newPlatformFrameCapturerForWindow(handle uintptr) (FrameCapturer, error) {
	windowID := C.long(handle)
	if !bool(C.isWindowValid(windowID)) {
		return nil, utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("window %d is invalid or closed", handle))
	}
	if !sckAvailable() {
		return nil, utils.WrapError(utils.ErrPlatformNotSupported,
			"FrameCapturer requires ScreenCaptureKit (macOS 14+)")
	}

	// Recover the owning PID and title so the session reports complete WindowInfo.
	// Not fatal if they cannot be found — under Wine the PID is the wrapper's
	// anyway and the window ID is what actually matters — but a session with an
	// empty title is confusing to log against.
	var pid uint32
	var title string
	if count := int(C.enumerateWindows(C.bool(true))); count > 0 {
		titleBuf := (*C.char)(C.malloc(titleBufferSize))
		defer C.free(unsafe.Pointer(titleBuf))

		for i := 0; i < count; i++ {
			var id C.long
			var p, layer, x, y, w, h C.int
			if !bool(C.windowSnapshotAt(C.int(i), &id, &p, &layer, &x, &y, &w, &h,
				titleBuf, titleBufferSize, nil, 0)) {
				continue
			}
			if id == windowID {
				pid = uint32(p)
				title = C.GoString(titleBuf)
				break
			}
		}
	}

	fc := &darwinFrameCapturer{
		pid:      pid,
		windowID: windowID,
		info: WindowInfo{
			Handle:      handle,
			Title:       title,
			PID:         pid,
			ScaleFactor: float64(C.getWindowScaleFactor(windowID)),
		},
	}
	if !fc.syncBounds() {
		return nil, utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("could not read bounds of window %d", handle))
	}
	return fc, nil
}
