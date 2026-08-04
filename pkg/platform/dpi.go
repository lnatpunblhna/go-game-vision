// Package platform holds process-wide, OS-level setup that must happen before
// any window or input API is touched — regardless of which of this project's
// packages the caller actually imports.
//
// It exists because of a subtle ordering hazard. Making a Windows process DPI
// aware changes what GetWindowRect, GetSystemMetrics and SendInput report, and
// the setting is process-wide and one-shot: whoever calls first wins, and later
// calls are ignored. Previously this lived in an init() inside pkg/mouse, which
// meant a program that imported only pkg/capture silently ran without DPI
// awareness and read back DPI-virtualised window rectangles. Putting it in its
// own leaf package that both pkg/capture and pkg/mouse depend on removes that
// import-order coupling.
package platform

import "sync"

var dpiOnce sync.Once

// EnsureDPIAware makes the current process DPI aware, idempotently.
//
// Safe to call from anywhere, any number of times, from multiple goroutines —
// only the first call does work. On non-Windows platforms this is a no-op:
// macOS reports window geometry in logical points and handles Retina scaling
// through WindowInfo.ScaleFactor instead.
func EnsureDPIAware() {
	dpiOnce.Do(setDPIAware)
}

// DPIAwarenessMode describes how the process ended up being made DPI aware.
// Useful for diagnostics when click coordinates look off on a scaled display.
type DPIAwarenessMode string

const (
	// DPIAwarenessUnsupported means the platform needs no DPI opt-in (macOS).
	DPIAwarenessUnsupported DPIAwarenessMode = "unsupported"
	// DPIAwarenessPerMonitorV2 is the preferred mode: correct on mixed-DPI
	// multi-monitor setups (Windows 10 1703+).
	DPIAwarenessPerMonitorV2 DPIAwarenessMode = "per-monitor-v2"
	// DPIAwarenessPerMonitor is the Windows 8.1 fallback.
	DPIAwarenessPerMonitor DPIAwarenessMode = "per-monitor"
	// DPIAwarenessSystem is the legacy Vista-era fallback: correct only while
	// every monitor shares the primary display's scale factor.
	DPIAwarenessSystem DPIAwarenessMode = "system"
	// DPIAwarenessFailed means every opt-in attempt failed.
	DPIAwarenessFailed DPIAwarenessMode = "failed"
)

// DPIAwareness reports which DPI awareness mode was achieved. It implies
// EnsureDPIAware, so callers can use it as a combined setup-and-report call.
func DPIAwareness() DPIAwarenessMode {
	EnsureDPIAware()
	return dpiMode
}

// init opts in as early as possible: merely importing this package is enough,
// which protects callers who reach a Windows API through some path we did not
// anticipate.
func init() {
	EnsureDPIAware()
}
