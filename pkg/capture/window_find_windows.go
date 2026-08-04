//go:build windows

package capture

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"golang.org/x/sys/windows"
)

var (
	procGetWindowLongW      = user32.NewProc("GetWindowLongW")
	procGetParentWindow     = user32.NewProc("GetParent")
	procGetClassNameW       = user32.NewProc("GetClassNameW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
)

// Window style bits used for filtering.
const (
	gwlExStyle       = -20
	wsExToolWindow   = 0x00000080 // tool windows are not ordinary app windows
	wsExNoActivate   = 0x08000000
	classNameBufSize = 256
)

// findWindowsPlatform enumerates top-level Windows windows and applies the query.
func findWindowsPlatform(query *WindowQuery) ([]FoundWindow, error) {
	capturer := &WindowsCapture{}
	var found []FoundWindow

	callback := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		// Skip child windows: EnumWindows only yields top-level windows, but a
		// window owned by another is still not the main game window.
		if parent, _, _ := procGetParentWindow.Call(hwnd); parent != 0 {
			return 1
		}

		if !query.IncludeOffscreen && !capturer.isWindowVisible(hwnd) {
			return 1
		}

		// Windows has no window "layer", so the nearest equivalent filter is the
		// extended style: tool windows are the palettes and helpers that would
		// otherwise pollute a title search.
		layer := 0
		if !query.IncludeAllLayers {
			// GWL_EXSTYLE is -20, and a negative constant cannot be converted to
			// uintptr at compile time; going through a variable performs the
			// sign-extending runtime conversion the API expects.
			index := int32(gwlExStyle)
			exStyle, _, _ := procGetWindowLongW.Call(hwnd, uintptr(index))
			if exStyle&wsExToolWindow != 0 {
				return 1
			}
		}

		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

		info := WindowInfo{
			Handle: hwnd,
			Title:  capturer.getWindowTitle(hwnd),
			PID:    pid,
			Rect:   capturer.getWindowRect(hwnd),
			// Per-monitor DPI aware, so rect and captured pixels agree.
			ScaleFactor: 1.0,
			IsHidden:    !capturer.isWindowVisible(hwnd),
		}

		owner := processImageName(pid)
		if !query.matches(&info, owner, layer) {
			return 1
		}

		found = append(found, FoundWindow{
			WindowInfo: info,
			OwnerName:  owner,
			Layer:      layer,
		})
		return 1
	})

	ret, _, err := procEnumWindows.Call(callback, 0)
	if ret == 0 && len(found) == 0 {
		return nil, utils.WrapError(err, "EnumWindows failed")
	}
	return found, nil
}

// processImageName returns the executable name owning a PID, or "".
//
// Provides the OwnerContains criterion its value on Windows: the window title
// comes from the application, while this comes from the binary, so the two
// together disambiguate cases where either alone is ambiguous.
func processImageName(pid uint32) string {
	if pid == 0 {
		return ""
	}

	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// Expected for system and elevated processes; not worth reporting.
		return ""
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}

// newPlatformFrameCapturerForWindow builds a session bound to a specific HWND.
func newPlatformFrameCapturerForWindow(handle uintptr) (FrameCapturer, error) {
	if handle == 0 {
		return nil, utils.WrapError(utils.ErrInvalidParameter, "window handle is zero")
	}

	capturer := &WindowsCapture{}
	var pid uint32
	procGetWindowThreadProcessId.Call(handle, uintptr(unsafe.Pointer(&pid)))

	fc := &windowsFrameCapturer{
		pid:            pid,
		hwnd:           handle,
		usePrintWindow: true,
		info: WindowInfo{
			Handle:      handle,
			Title:       capturer.getWindowTitle(handle),
			PID:         pid,
			ScaleFactor: 1.0,
		},
	}
	if err := fc.syncBounds(); err != nil {
		return nil, utils.WrapError(err, fmt.Sprintf("could not read bounds of window 0x%X", handle))
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
