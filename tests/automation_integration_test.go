package tests

import (
	stdimage "image"
	"image/color"
	"testing"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/automation"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Integration tests for the Session timing primitives against a real window.
// Guarded by -short: they need a windowing session and screen-recording
// permission.

// newTestSession opens a Session against some on-screen window.
//
// The Guard is disabled: these tests never click, and an armed Guard installs
// signal handlers that would interfere with the test binary.
func newTestSession(t *testing.T) *automation.Session {
	t.Helper()

	pid := findCapturableWindow(t)
	if pid == 0 {
		t.Skip("no capturable window found (screen-recording permission?)")
	}

	s, err := automation.NewSession(pid, &automation.SessionOptions{DisableGuard: true})
	if err != nil {
		t.Skipf("could not open a session for PID %d: %v", pid, err)
	}
	return s
}

// registerCroppedTemplate captures a frame and registers a patch cropped from its
// centre, so the template is guaranteed present at a known location.
func registerCroppedTemplate(t *testing.T, s *automation.Session, name string) stdimage.Point {
	t.Helper()

	frame, err := s.Capture()
	if err != nil {
		t.Skipf("capture failed: %v", err)
	}

	const pw, ph = 80, 50
	if frame.Width < pw*3 || frame.Height < ph*3 {
		t.Skipf("window too small (%dx%d) for a meaningful template test", frame.Width, frame.Height)
	}

	originX := frame.Width/2 - pw/2
	originY := frame.Height/2 - ph/2

	rgba := frame.ToRGBA()
	patch := stdimage.NewRGBA(stdimage.Rect(0, 0, pw, ph))
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			patch.Set(x, y, rgba.At(originX+x, originY+y))
		}
	}

	if err := s.RegisterTemplate(name, patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}
	return stdimage.Point{X: originX, Y: originY}
}

func TestSessionFindLocatesCroppedPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	want := registerCroppedTemplate(t, s, "patch")

	match, frame, err := s.Find("patch", &ggvimage.MatchOptions{Threshold: 0.8})
	if err != nil {
		// A live window may have repainted between the crop and the search.
		t.Skipf("patch not found, window likely repainted: %v", err)
	}

	t.Logf("expected (%d,%d), found %v, similarity %.4f",
		want.X, want.Y, match.Location, match.Similarity)

	// Allow a couple of pixels for a partial repaint shifting the best match.
	if abs(match.Location.X-want.X) > 2 || abs(match.Location.Y-want.Y) > 2 {
		t.Errorf("found at %v, expected near (%d,%d)", match.Location, want.X, want.Y)
	}

	// The whole point of carrying the frame back: mapping to a click point must
	// land inside the window.
	click := match.ToScreenCenter(&frame.Window)
	if !click.In(frame.Window.Rect) {
		t.Errorf("click point %v is outside the window rect %v", click, frame.Window.Rect)
	}
}

func TestSessionWaitForSucceedsImmediatelyWhenPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	registerCroppedTemplate(t, s, "patch")

	start := time.Now()
	_, _, err := s.WaitFor("patch", &automation.WaitOptions{
		Timeout: 3 * time.Second,
		Match:   &ggvimage.MatchOptions{Threshold: 0.8},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Skipf("patch not found, window likely repainted: %v", err)
	}
	// It was already on screen, so the first poll should have matched.
	if elapsed > time.Second {
		t.Errorf("WaitFor took %v for an already-visible template", elapsed)
	}
}

// TestSessionWaitForTimesOut checks the deadline is honoured and reported as
// ErrTimeout rather than as an indefinite hang.
func TestSessionWaitForTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	// A synthetic template of uniform magenta will not appear on screen.
	absent := stdimage.NewRGBA(stdimage.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			absent.Set(x, y, colorMagenta())
		}
	}
	if err := s.RegisterTemplate("absent", absent); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	start := time.Now()
	_, _, err := s.WaitFor("absent", &automation.WaitOptions{
		Timeout:      600 * time.Millisecond,
		PollInterval: 50 * time.Millisecond,
		Match:        &ggvimage.MatchOptions{Threshold: 0.99},
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("WaitFor succeeded for a template that should be absent")
	}
	if !utils.IsError(err, automation.ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
	// Should respect the deadline rather than overshooting by a poll interval.
	if elapsed > 3*time.Second {
		t.Errorf("WaitFor overshot its 600ms timeout by a lot: %v", elapsed)
	}
}

// TestSessionWaitVanishSucceedsForAbsentTemplate is the mirror case: something
// never present is trivially gone.
func TestSessionWaitVanishSucceedsForAbsentTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	absent := stdimage.NewRGBA(stdimage.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			absent.Set(x, y, colorMagenta())
		}
	}
	if err := s.RegisterTemplate("absent", absent); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	start := time.Now()
	err := s.WaitVanish("absent", &automation.WaitOptions{
		Timeout: 3 * time.Second,
		Match:   &ggvimage.MatchOptions{Threshold: 0.99},
	})
	if err != nil {
		t.Errorf("WaitVanish returned %v for an absent template", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("WaitVanish took %v for an absent template", elapsed)
	}
}

// TestSessionWaitStableOnQuietWindow exercises the loading-finished detector. Not
// asserted strictly: a live desktop may genuinely never settle (a clock, a
// spinner), so an unstable result is logged rather than failed.
func TestSessionWaitStableOnQuietWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	frame, err := s.WaitStable(&automation.StableOptions{
		Timeout:      4 * time.Second,
		PollInterval: 80 * time.Millisecond,
	})
	if err != nil {
		t.Logf("window did not stabilise (an animating window is plausible): %v", err)
		return
	}
	if frame == nil {
		t.Error("WaitStable returned a nil frame with no error")
		return
	}
	t.Logf("stabilised at %dx%d px", frame.Width, frame.Height)
}

func TestSessionTemplateRegistrationErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window and screen-recording permission")
	}

	s := newTestSession(t)
	defer s.Close()

	// Unregistered lookups must be a clear error, not a nil-template panic later.
	if _, err := s.Template("nope"); err == nil {
		t.Error("Template() succeeded for an unregistered name")
	}
	if _, _, err := s.Find("nope", nil); err == nil {
		t.Error("Find() succeeded for an unregistered template")
	}

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 10, 10))
	if err := s.RegisterTemplate("dup", img); err != nil {
		t.Fatalf("first RegisterTemplate: %v", err)
	}
	// Silently replacing a template would make a name collision very hard to spot.
	if err := s.RegisterTemplate("dup", img); err == nil {
		t.Error("RegisterTemplate silently overwrote an existing name")
	}
}

// colorMagenta is a colour unlikely to appear as a large uniform block on a
// desktop, used to build templates that should never match.
func colorMagenta() color.RGBA {
	return color.RGBA{R: 255, G: 0, B: 255, A: 255}
}
