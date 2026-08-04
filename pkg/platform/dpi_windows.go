//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
)

// DPI awareness constants.
//
// dpiAwarenessContextPerMonitorAwareV2 is passed as a pseudo-handle, hence the
// negative value cast to uintptr — that is genuinely how the Win32 API defines
// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 ((DPI_AWARENESS_CONTEXT)-4).
const (
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(0) - 3 // (HANDLE)-4
	processPerMonitorDPIAware            = 2               // PROCESS_PER_MONITOR_DPI_AWARE
)

// dpiMode records which fallback rung we landed on.
var dpiMode = DPIAwarenessFailed

// setDPIAware walks three generations of Win32 DPI APIs, newest first.
//
// Why bother with the ladder instead of just calling SetProcessDPIAware: the
// legacy call only makes the process *system* DPI aware, which is correct only
// while every monitor shares the primary display's scale factor. Plug in a
// second monitor at a different scale — a completely ordinary setup — and
// window rectangles on the secondary display come back virtualised, so every
// computed click lands in the wrong place. Per-monitor-v2 is the only mode that
// reports true physical pixels everywhere.
func setDPIAware() {
	// Windows 10 1703+ — per-monitor v2, the only fully correct option.
	if procSetProcessDpiAwarenessContext.Find() == nil {
		if ret, _, _ := procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2); ret != 0 {
			dpiMode = DPIAwarenessPerMonitorV2
			return
		}
		// A zero return here is expected (ERROR_ACCESS_DENIED) when awareness
		// was already set — by a manifest, or by an earlier caller. Either way
		// there is nothing left for us to do, and trying the older APIs would
		// only fail the same way.
	}

	// Windows 8.1+ — per-monitor v1. Returns an HRESULT: S_OK == 0.
	if procSetProcessDpiAwareness.Find() == nil {
		if ret, _, _ := procSetProcessDpiAwareness.Call(processPerMonitorDPIAware); ret == 0 {
			dpiMode = DPIAwarenessPerMonitor
			return
		}
	}

	// Vista+ — system DPI aware. Better than nothing on a single-monitor box.
	if procSetProcessDPIAware.Find() == nil {
		if ret, _, _ := procSetProcessDPIAware.Call(); ret != 0 {
			dpiMode = DPIAwarenessSystem
			return
		}
	}
}
