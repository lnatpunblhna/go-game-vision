package tests

import (
	"sort"
	"testing"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/process"
)

// Integration tests for the capture path against a real on-screen window.
//
// Guarded by -short because they need a windowing session and, on macOS, Screen
// Recording permission. CI runs them on the macOS job only.

// findWindowedProcess returns the PID of the process with the largest window, or
// 0. Iterating and probing is the only portable way to find one — there is no
// cross-platform "give me a window" call, and hard-coding a process name (the
// existing tests look for "Explorer") makes the test a permanent skip on the
// other platform.
//
// Largest rather than first: the first hit is often a transient system window
// (NotificationCenter, ControlCenter) that may close mid-test or that
// ScreenCaptureKit declines to share, producing confusing failures.
func findWindowedProcess(t *testing.T) (uint32, *capture.WindowInfo) {
	t.Helper()

	manager := process.NewProcessManager()
	processes, err := manager.ListAllProcesses()
	if err != nil {
		t.Fatalf("failed to list processes: %v", err)
	}

	var bestPID uint32
	var bestInfo *capture.WindowInfo
	bestArea := 0

	for _, proc := range processes {
		if proc.PID <= 100 {
			continue
		}
		info, err := capture.GetWindowInfoByPID(proc.PID)
		if err != nil || info == nil {
			continue
		}
		// A usable test target needs a real, non-degenerate window.
		if info.Rect.Dx() < 100 || info.Rect.Dy() < 100 {
			continue
		}
		if area := info.Rect.Dx() * info.Rect.Dy(); area > bestArea {
			bestArea, bestPID, bestInfo = area, proc.PID, info
		}
	}
	return bestPID, bestInfo
}

// findCapturableWindow returns a PID whose window a FrameCapturer can actually
// capture, trying candidates in descending size order.
//
// Needed because window existence and window capturability are different
// questions: ScreenCaptureKit refuses some system-owned windows, and a
// FrameCapturer has no subprocess fallback to hide that behind.
func findCapturableWindow(t *testing.T) uint32 {
	t.Helper()

	manager := process.NewProcessManager()
	processes, err := manager.ListAllProcesses()
	if err != nil {
		t.Fatalf("failed to list processes: %v", err)
	}

	type candidate struct {
		pid  uint32
		area int
	}
	var candidates []candidate

	for _, proc := range processes {
		if proc.PID <= 100 {
			continue
		}
		info, err := capture.GetWindowInfoByPID(proc.PID)
		if err != nil || info == nil {
			continue
		}
		if info.Rect.Dx() < 300 || info.Rect.Dy() < 200 {
			continue
		}
		candidates = append(candidates, candidate{proc.PID, info.Rect.Dx() * info.Rect.Dy()})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].area > candidates[j].area })

	for _, c := range candidates {
		fc, err := capture.NewFrameCapturer(c.pid)
		if err != nil {
			continue
		}
		_, err = fc.Capture()
		fc.Close()
		if err == nil {
			return c.pid
		}
		t.Logf("PID %d has a window but cannot be captured: %v", c.pid, err)
	}
	return 0
}

// TestCaptureMatchesReportedScale pins the invariant that makes click
// coordinates correct: the captured image must be exactly the window's size
// multiplied by the reported ScaleFactor.
//
// If this drifts, every template match silently maps to the wrong click point —
// which is precisely the bug this was written to catch. It once failed at 2.127x
// instead of 2.0x because `screencapture` was including the window's drop
// shadow, inflating the frame by ~34pt per side.
func TestCaptureMatchesReportedScale(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	pid, info := findWindowedProcess(t)
	if pid == 0 {
		t.Skip("no process with a usable window found")
	}

	img, err := capture.CaptureWindowByPID(pid, nil)
	if err != nil {
		t.Skipf("capture failed for PID %d (permission not granted?): %v", pid, err)
	}

	scale := info.ScaleOrDefault()
	wantW := int(float64(info.Rect.Dx()) * scale)
	wantH := int(float64(info.Rect.Dy()) * scale)
	gotW := img.Bounds().Dx()
	gotH := img.Bounds().Dy()

	t.Logf("PID %d: rect=%v scale=%.2f -> want %dx%d px, got %dx%d px",
		pid, info.Rect, scale, wantW, wantH, gotW, gotH)

	// Allow a couple of pixels for the window server rounding a fractional
	// logical size, but nothing like a whole shadow border or a 2x factor.
	const tolerance = 3
	if abs(gotW-wantW) > tolerance || abs(gotH-wantH) > tolerance {
		t.Errorf("captured size %dx%d disagrees with rect %v x scale %.2f (want %dx%d). "+
			"Clicks derived from matches in this image will be offset.",
			gotW, gotH, info.Rect, scale, wantW, wantH)
	}
}

// TestCaptureIsNotBlank guards against the failure mode where the capture API
// reports success but hands back an empty frame — which template matching would
// report as "just no match" rather than as a broken capture.
func TestCaptureIsNotBlank(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	pid, _ := findWindowedProcess(t)
	if pid == 0 {
		t.Skip("no process with a usable window found")
	}

	img, err := capture.CaptureWindowByPID(pid, nil)
	if err != nil {
		t.Skipf("capture failed for PID %d: %v", pid, err)
	}

	b := img.Bounds()
	var nonBlack, opaque, sampled int
	for y := b.Min.Y; y < b.Max.Y; y += 7 {
		for x := b.Min.X; x < b.Max.X; x += 7 {
			r, g, bl, a := img.At(x, y).RGBA()
			sampled++
			if r > 0x0800 || g > 0x0800 || bl > 0x0800 {
				nonBlack++
			}
			if a > 0x8000 {
				opaque++
			}
		}
	}

	if sampled == 0 {
		t.Fatal("captured image has no pixels")
	}
	nonBlackPct := 100 * float64(nonBlack) / float64(sampled)
	opaquePct := 100 * float64(opaque) / float64(sampled)
	t.Logf("sampled=%d nonBlack=%.1f%% opaque=%.1f%%", sampled, nonBlackPct, opaquePct)

	// A window that is genuinely mostly black is possible but unusual; 1% is a
	// floor that only an essentially empty frame would fail.
	if nonBlackPct < 1.0 {
		t.Errorf("captured frame looks blank (%.1f%% non-black pixels)", nonBlackPct)
	}
	if opaquePct < 1.0 {
		t.Errorf("captured frame looks fully transparent (%.1f%% opaque pixels)", opaquePct)
	}
}

// TestCaptureThroughput records how fast the capture path actually is. It does
// not assert a threshold — CI machines vary too much for that to be stable — but
// the logged figure is what tells you whether an automation loop is viable: the
// screencapture subprocess path managed 5-7 fps, ScreenCaptureKit ~20 fps.
func TestCaptureThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	pid, _ := findWindowedProcess(t)
	if pid == 0 {
		t.Skip("no process with a usable window found")
	}

	// Warm up: the first capture pays for enumerating shareable content.
	if _, err := capture.CaptureWindowByPID(pid, nil); err != nil {
		t.Skipf("capture failed for PID %d: %v", pid, err)
	}

	const n = 5
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := capture.CaptureWindowByPID(pid, nil); err != nil {
			t.Fatalf("capture %d failed: %v", i, err)
		}
	}
	avg := time.Since(start) / n
	t.Logf("average capture: %v (%.1f fps)", avg.Round(time.Millisecond),
		float64(time.Second)/float64(avg))
}
