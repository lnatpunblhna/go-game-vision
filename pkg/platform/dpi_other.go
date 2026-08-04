//go:build !windows

package platform

// dpiMode is fixed on non-Windows platforms: there is no process-wide DPI
// opt-in to perform.
var dpiMode = DPIAwarenessUnsupported

// setDPIAware is a no-op outside Windows.
//
// macOS has no equivalent process-wide switch. Window geometry from
// CGWindowListCopyWindowInfo and click coordinates for CGEventPost are both
// always in logical points; the pixel/point mismatch shows up only against
// captured images, and is handled per-window by capture.WindowInfo.ScaleFactor
// rather than by a global mode change.
func setDPIAware() {}
