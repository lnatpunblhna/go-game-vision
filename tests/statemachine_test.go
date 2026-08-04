package tests

import (
	"errors"
	"fmt"
	stdimage "image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/automation"
	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// State machine tests driven by a scripted frame source, so the decision logic is
// verified without a window, a display or permissions. This is the reason
// NewSessionWithCapturer exists.

// fakeCapturer serves frames from a script. The frame returned is whatever
// current() yields, so a test can change the "screen" between iterations.
type fakeCapturer struct {
	mu      sync.Mutex
	frame   *capture.RawFrame
	err     error
	calls   int
	closed  bool
	onFrame func(call int) (*capture.RawFrame, error)
}

func (f *fakeCapturer) Capture() (*capture.RawFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.onFrame != nil {
		return f.onFrame(f.calls)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.frame, nil
}

func (f *fakeCapturer) Window() *capture.WindowInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.frame == nil {
		return &capture.WindowInfo{ScaleFactor: 1.0}
	}
	info := f.frame.Window
	return &info
}

func (f *fakeCapturer) Refresh() error { return nil }

func (f *fakeCapturer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeCapturer) setFrame(frame *capture.RawFrame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frame = frame
}

func (f *fakeCapturer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// markerFrame builds a frame containing one distinctive marker patch, plus the
// patch itself for registration as a template. Different ids give different
// markers, so states can be told apart.
func markerFrame(id byte) (*capture.RawFrame, *stdimage.RGBA) {
	const w, h = 240, 180
	frame, set := bgraFrame(w, h)

	// Background varies with id too, but the marker is what detection keys on.
	fillRect(set, stdimage.Rect(0, 0, w, h), 30, 30, 30)

	// A 40x30 marker at a fixed spot, with an id-dependent pattern.
	patch := stdimage.NewRGBA(stdimage.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			b := byte((int(id)*37 + x*5) % 256)
			g := byte((int(id)*91 + y*7) % 256)
			r := byte((int(id)*53 + x*3 + y*11) % 256)
			patch.Set(x, y, rgba(r, g, b))
			set(20+x, 20+y, b, g, r)
		}
	}
	return frame, patch
}

func newScriptedSession(t *testing.T, fc *fakeCapturer) *automation.Session {
	t.Helper()
	s, err := automation.NewSessionWithCapturer(fc, &automation.SessionOptions{
		DisableGuard: true,
	})
	if err != nil {
		t.Fatalf("NewSessionWithCapturer: %v", err)
	}
	return s
}

func TestMachineReachesTerminalState(t *testing.T) {
	frameA, patchA := markerFrame(1)
	frameB, patchB := markerFrame(2)

	fc := &fakeCapturer{frame: frameA}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("markerA", patchA); err != nil {
		t.Fatalf("RegisterTemplate A: %v", err)
	}
	if err := s.RegisterTemplate("markerB", patchB); err != nil {
		t.Fatalf("RegisterTemplate B: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})

	err := m.AddStates(
		&automation.State{
			Name:   "start",
			Detect: automation.TemplateVisible("markerA", &ggvimage.MatchOptions{Threshold: 0.9}),
			Action: func(c *automation.Context) (string, error) {
				// "Perform an action" by switching the screen the fake serves.
				fc.setFrame(frameB)
				return "finish", nil
			},
		},
		&automation.State{
			Name:     "finish",
			Detect:   automation.TemplateVisible("markerB", &ggvimage.MatchOptions{Threshold: 0.9}),
			Terminal: true,
		},
	)
	if err != nil {
		t.Fatalf("AddStates: %v", err)
	}

	if err := m.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	visits := m.Visits()
	if visits["start"] != 1 {
		t.Errorf("start visited %d times, want 1", visits["start"])
	}
	if visits["finish"] != 1 {
		t.Errorf("finish visited %d times, want 1", visits["finish"])
	}
}

func TestMachineStopsOnDone(t *testing.T) {
	frame, patch := markerFrame(3)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})

	actions := 0
	if err := m.AddState(&automation.State{
		Name:   "loop",
		Detect: automation.TemplateVisible("marker", &ggvimage.MatchOptions{Threshold: 0.9}),
		Action: func(c *automation.Context) (string, error) {
			actions++
			if actions >= 3 {
				return "", automation.Done
			}
			return "", nil
		},
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	if err := m.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if actions != 3 {
		t.Errorf("action ran %d times, want 3", actions)
	}
}

// TestMachineDetectionRespectsPriority is the overlapping-states case: a modal
// popup drawn over the game screen matches both detectors, and the popup must win
// or the script keeps trying to play through it.
func TestMachineDetectionRespectsPriority(t *testing.T) {
	frame, patch := markerFrame(4)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})

	var acted string
	alwaysTrue := func(c *automation.Context) (bool, error) { return true, nil }

	err := m.AddStates(
		&automation.State{
			Name: "low-priority", Priority: 1, Detect: alwaysTrue,
			Action: func(c *automation.Context) (string, error) {
				acted = "low-priority"
				return "", automation.Done
			},
		},
		&automation.State{
			Name: "high-priority", Priority: 100, Detect: alwaysTrue,
			Action: func(c *automation.Context) (string, error) {
				acted = "high-priority"
				return "", automation.Done
			},
		},
	)
	if err != nil {
		t.Fatalf("AddStates: %v", err)
	}

	if err := m.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if acted != "high-priority" {
		t.Errorf("acted on %q, want high-priority to win", acted)
	}
}

// TestMachineUnknownStateInvokesRecovery covers the fallback that keeps a script
// from spinning forever on a screen nobody anticipated.
func TestMachineUnknownStateInvokesRecovery(t *testing.T) {
	frame, patch := markerFrame(5)
	_, otherPatch := markerFrame(6)

	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	// Register a template for a marker that is NOT on screen, so nothing matches.
	if err := s.RegisterTemplate("absent", otherPatch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}
	if err := s.RegisterTemplate("present", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	recoveries := 0
	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:          5 * time.Second,
		IterationDelay:   time.Millisecond,
		MaxUnknownStreak: 5,
		OnUnknown: func(c *automation.Context) error {
			recoveries++
			// "Recover" by making the recognisable marker appear.
			if recoveries >= 2 {
				newFrame, _ := markerFrame(5)
				fc.setFrame(newFrame)
			}
			return nil
		},
	})

	if err := m.AddState(&automation.State{
		Name:   "known",
		Detect: automation.TemplateVisible("absent", &ggvimage.MatchOptions{Threshold: 0.99}),
		Action: func(c *automation.Context) (string, error) { return "", automation.Done },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	// Nothing will ever match, so this must terminate via the streak limit rather
	// than hang — and OnUnknown must have been given a chance first.
	err := m.Run()
	if err == nil {
		t.Fatal("Run succeeded despite no state ever matching")
	}
	if !errors.Is(err, automation.ErrUnknownState) {
		t.Errorf("error = %v, want ErrUnknownState", err)
	}
	if recoveries == 0 {
		t.Error("OnUnknown was never called")
	}
}

func TestMachineUnknownStateFailsWithoutRecovery(t *testing.T) {
	frame, _ := markerFrame(7)
	_, otherPatch := markerFrame(8)

	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("absent", otherPatch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:          5 * time.Second,
		IterationDelay:   time.Millisecond,
		MaxUnknownStreak: 3,
	})
	if err := m.AddState(&automation.State{
		Name:   "known",
		Detect: automation.TemplateVisible("absent", &ggvimage.MatchOptions{Threshold: 0.99}),
		Action: func(c *automation.Context) (string, error) { return "", automation.Done },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	start := time.Now()
	err := m.Run()
	if !errors.Is(err, automation.ErrUnknownState) {
		t.Errorf("error = %v, want ErrUnknownState", err)
	}
	// Must give up after the streak limit, not run to the 5s timeout.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v to give up; the unknown streak limit should have fired sooner", elapsed)
	}
}

// TestMachineStuckStateDetected covers the other silent-hang mode: an action that
// runs cleanly but never changes the screen.
func TestMachineStuckStateDetected(t *testing.T) {
	frame, patch := markerFrame(9)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        10 * time.Second,
		IterationDelay: time.Millisecond,
		MaxStateStreak: 4,
	})
	if err := m.AddState(&automation.State{
		Name:   "spinning",
		Detect: automation.TemplateVisible("marker", &ggvimage.MatchOptions{Threshold: 0.9}),
		// Never changes anything, so the state repeats forever.
		Action: func(c *automation.Context) (string, error) { return "", nil },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	err := m.Run()
	if !errors.Is(err, automation.ErrStuck) {
		t.Errorf("error = %v, want ErrStuck", err)
	}
}

func TestMachineStuckHandlerCanRecover(t *testing.T) {
	frameA, patchA := markerFrame(10)
	frameB, patchB := markerFrame(11)

	fc := &fakeCapturer{frame: frameA}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("a", patchA); err != nil {
		t.Fatalf("RegisterTemplate a: %v", err)
	}
	if err := s.RegisterTemplate("b", patchB); err != nil {
		t.Fatalf("RegisterTemplate b: %v", err)
	}

	stuckCalls := 0
	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        10 * time.Second,
		IterationDelay: time.Millisecond,
		MaxStateStreak: 3,
		OnStuck: func(c *automation.Context) error {
			stuckCalls++
			fc.setFrame(frameB) // unstick by changing the screen
			return nil
		},
	})

	err := m.AddStates(
		&automation.State{
			Name:   "stuck",
			Detect: automation.TemplateVisible("a", &ggvimage.MatchOptions{Threshold: 0.9}),
			Action: func(c *automation.Context) (string, error) { return "", nil },
		},
		&automation.State{
			Name:     "recovered",
			Detect:   automation.TemplateVisible("b", &ggvimage.MatchOptions{Threshold: 0.9}),
			Terminal: true,
		},
	)
	if err != nil {
		t.Fatalf("AddStates: %v", err)
	}

	if err := m.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stuckCalls == 0 {
		t.Error("OnStuck was never called")
	}
}

// TestMachineExpectedStateMismatchIsReported checks the declared-transition check:
// silently looping until the global timeout tells the author nothing, whereas this
// names the state whose action did not do what it claimed.
func TestMachineExpectedStateMismatchIsReported(t *testing.T) {
	frame, patch := markerFrame(12)
	_, otherPatch := markerFrame(13)

	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("here", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}
	if err := s.RegisterTemplate("elsewhere", otherPatch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})

	err := m.AddStates(
		&automation.State{
			Name:   "first",
			Detect: automation.TemplateVisible("here", &ggvimage.MatchOptions{Threshold: 0.9}),
			// Claims "second" comes next, but the screen never changes.
			Action: func(c *automation.Context) (string, error) { return "second", nil },
		},
		&automation.State{
			Name:     "second",
			Detect:   automation.TemplateVisible("elsewhere", &ggvimage.MatchOptions{Threshold: 0.99}),
			Terminal: true,
		},
	)
	if err != nil {
		t.Fatalf("AddStates: %v", err)
	}

	runErr := m.Run()
	if runErr == nil {
		t.Fatal("Run succeeded despite the declared transition not happening")
	}
	// The message must name both the expectation and what was actually seen.
	msg := runErr.Error()
	if !contains(msg, "second") || !contains(msg, "first") {
		t.Errorf("error %q should name both the expected and actual state", msg)
	}
}

func TestMachineTimeout(t *testing.T) {
	frame, patch := markerFrame(14)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        250 * time.Millisecond,
		IterationDelay: time.Millisecond,
		// Set high so the stuck detector does not fire before the timeout.
		MaxStateStreak: 100000,
	})
	if err := m.AddState(&automation.State{
		Name:   "forever",
		Detect: automation.TemplateVisible("marker", &ggvimage.MatchOptions{Threshold: 0.9}),
		Action: func(c *automation.Context) (string, error) { return "", nil },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	start := time.Now()
	err := m.Run()
	if !utils.IsError(err, automation.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run took %v, expected to stop near its 250ms timeout", elapsed)
	}
}

// TestMachinePersistentCaptureFailureTerminates guards against the loop treating
// a permanently broken capture as merely "not recognised yet" and spinning to the
// global timeout.
func TestMachinePersistentCaptureFailureTerminates(t *testing.T) {
	fc := &fakeCapturer{err: fmt.Errorf("capture is broken")}
	s := newScriptedSession(t, fc)
	defer s.Close()

	frame, patch := markerFrame(15)
	_ = frame
	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:          10 * time.Second,
		IterationDelay:   time.Millisecond,
		MaxUnknownStreak: 4,
	})
	if err := m.AddState(&automation.State{
		Name:   "any",
		Detect: automation.TemplateVisible("marker", nil),
		Action: func(c *automation.Context) (string, error) { return "", automation.Done },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	start := time.Now()
	if err := m.Run(); err == nil {
		t.Error("Run succeeded despite every capture failing")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v to give up on a broken capture", elapsed)
	}
	if fc.callCount() < 4 {
		t.Errorf("capture attempted %d times, expected at least the streak limit", fc.callCount())
	}
}

func TestMachineAddStateValidation(t *testing.T) {
	fc := &fakeCapturer{frame: func() *capture.RawFrame { f, _ := markerFrame(16); return f }()}
	s := newScriptedSession(t, fc)
	defer s.Close()

	m := automation.NewMachine(s, nil)
	detect := func(c *automation.Context) (bool, error) { return false, nil }
	action := func(c *automation.Context) (string, error) { return "", nil }

	if err := m.AddState(nil); err == nil {
		t.Error("expected an error for a nil state")
	}
	if err := m.AddState(&automation.State{Detect: detect, Action: action}); err == nil {
		t.Error("expected an error for an empty state name")
	}
	if err := m.AddState(&automation.State{Name: "no-detect", Action: action}); err == nil {
		t.Error("expected an error for a state without Detect")
	}
	// A non-terminal state with no action would be detected forever with nothing
	// to advance it — the definition of a stuck machine, so reject it up front.
	if err := m.AddState(&automation.State{Name: "no-action", Detect: detect}); err == nil {
		t.Error("expected an error for a non-terminal state without Action")
	}
	// Terminal states legitimately need no action.
	if err := m.AddState(&automation.State{Name: "terminal", Detect: detect, Terminal: true}); err != nil {
		t.Errorf("terminal state without Action rejected: %v", err)
	}
	if err := m.AddState(&automation.State{Name: "terminal", Detect: detect, Terminal: true}); err == nil {
		t.Error("expected an error for a duplicate state name")
	}

	if err := automation.NewMachine(s, nil).Run(); err == nil {
		t.Error("expected an error running a machine with no states")
	}
}

func TestMachineUnregisteredNextStateIsReported(t *testing.T) {
	frame, patch := markerFrame(17)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})
	if err := m.AddState(&automation.State{
		Name:   "only",
		Detect: automation.TemplateVisible("marker", &ggvimage.MatchOptions{Threshold: 0.9}),
		Action: func(c *automation.Context) (string, error) { return "does-not-exist", nil },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	err := m.Run()
	if err == nil {
		t.Fatal("Run succeeded despite naming an unregistered next state")
	}
	if !contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q should name the unregistered state", err)
	}
}

func TestMachineDetectorErrorIsSurfaced(t *testing.T) {
	frame, _ := markerFrame(18)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	m := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:        5 * time.Second,
		IterationDelay: time.Millisecond,
	})
	// An unregistered template inside a detector is a script bug, not a
	// "screen not recognised" condition, so it must surface rather than be
	// swallowed as a non-match.
	if err := m.AddState(&automation.State{
		Name:   "broken",
		Detect: automation.TemplateVisible("never-registered", nil),
		Action: func(c *automation.Context) (string, error) { return "", automation.Done },
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	err := m.Run()
	if err == nil {
		t.Fatal("Run succeeded despite a broken detector")
	}
	if !contains(err.Error(), "never-registered") {
		t.Errorf("error %q should name the missing template", err)
	}
}

func TestMachineDetectWithoutActing(t *testing.T) {
	frame, patch := markerFrame(19)
	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("marker", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	m := automation.NewMachine(s, nil)
	if err := m.AddState(&automation.State{
		Name:     "here",
		Detect:   automation.TemplateVisible("marker", &ggvimage.MatchOptions{Threshold: 0.9}),
		Terminal: true,
	}); err != nil {
		t.Fatalf("AddState: %v", err)
	}

	name, got, err := m.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if name != "here" {
		t.Errorf("Detect() = %q, want %q", name, "here")
	}
	if got == nil {
		t.Error("Detect returned a nil frame")
	}
}

func TestAnyAllVisibleDetectors(t *testing.T) {
	frame, patch := markerFrame(20)
	_, absentPatch := markerFrame(21)

	fc := &fakeCapturer{frame: frame}
	s := newScriptedSession(t, fc)
	defer s.Close()

	if err := s.RegisterTemplate("present", patch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}
	if err := s.RegisterTemplate("absent", absentPatch); err != nil {
		t.Fatalf("RegisterTemplate: %v", err)
	}

	strict := &ggvimage.MatchOptions{Threshold: 0.99}
	loose := &ggvimage.MatchOptions{Threshold: 0.9}

	// Run each detector through a one-shot machine so it gets a real Context.
	check := func(name string, detect func(*automation.Context) (bool, error)) bool {
		m := automation.NewMachine(s, &automation.MachineOptions{
			Timeout:        2 * time.Second,
			IterationDelay: time.Millisecond,
		})
		if err := m.AddState(&automation.State{
			Name: name, Detect: detect, Terminal: true,
		}); err != nil {
			t.Fatalf("AddState: %v", err)
		}
		got, _, err := m.Detect()
		if err != nil {
			t.Fatalf("Detect(%s): %v", name, err)
		}
		return got == name
	}

	if !check("present-visible", automation.TemplateVisible("present", loose)) {
		t.Error("TemplateVisible should match a present template")
	}
	if !check("absent-gone", automation.TemplateAbsent("absent", strict)) {
		t.Error("TemplateAbsent should match an absent template")
	}
	if check("all-both", automation.AllVisible([]string{"present", "absent"}, strict)) {
		t.Error("AllVisible should not match when one template is absent")
	}
	if !check("any-one", automation.AnyVisible([]string{"absent", "present"}, loose)) {
		t.Error("AnyVisible should match when one template is present")
	}
	if check("all-empty", automation.AllVisible(nil, loose)) {
		t.Error("AllVisible with no names should not match")
	}
}

func TestRegionOf(t *testing.T) {
	frame, _ := markerFrame(22) // 240x180

	// Middle horizontal band: full width, middle third.
	got := automation.RegionOf(frame, 0, 1.0/3, 1.0, 1.0/3)
	want := stdimage.Rect(0, 60, 240, 120)
	if got != want {
		t.Errorf("RegionOf = %v, want %v", got, want)
	}

	if r := automation.RegionOf(nil, 0, 0, 1, 1); !r.Empty() {
		t.Errorf("RegionOf(nil) = %v, want empty", r)
	}
}

// contains is a tiny substring helper, kept local to avoid pulling in strings for
// one call site per assertion.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// rgba builds an opaque colour from R, G, B components.
func rgba(r, g, b byte) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
