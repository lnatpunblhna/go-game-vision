// Package automation ties capture, matching, input and the emergency stop into
// the loop-shaped API that automation scripts actually need.
//
// The lower-level packages each solve one problem and leave the wiring to the
// caller: pkg/capture produces frames, pkg/image finds things in them,
// pkg/mouse clicks, pkg/safety aborts. Doing that wiring by hand in every script
// means re-deriving the same details every time — which template Mat to reuse,
// which coordinate space a match is in, whether a sleep can be interrupted.
// A Session owns those decisions once.
package automation

import (
	"errors"
	"fmt"
	stdimage "image"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/safety"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// ErrTimeout is returned by the Wait* primitives when the condition does not
// occur within the deadline.
var ErrTimeout = errors.New("timed out waiting for condition")

// SessionOptions configures a Session.
type SessionOptions struct {
	// Guard is the emergency stop. If nil, a default Guard is created and owned
	// by the Session.
	//
	// Automation that drives the real cursor must be interruptible; see
	// pkg/safety for why this is not optional on macOS.
	Guard *safety.Guard

	// DisableGuard skips creating a Guard entirely. Only appropriate for
	// automation that cannot move the cursor — for example a Windows-only script
	// using PostMessage clicks exclusively.
	DisableGuard bool

	// DefaultThreshold is the similarity threshold used when a match does not
	// specify one. Zero means image.DefaultMatchThreshold.
	DefaultThreshold float64

	// PollInterval is how often the Wait* primitives re-capture. Zero means
	// DefaultPollInterval.
	PollInterval time.Duration
}

// Defaults for SessionOptions.
const DefaultPollInterval = 100 * time.Millisecond

func (o *SessionOptions) pollInterval() time.Duration {
	if o == nil || o.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return o.PollInterval
}

// Session is an automation session bound to one window.
//
// Not safe for concurrent use: it owns a reused frame buffer and reused OpenCV
// matrices. Use one Session per automation goroutine.
type Session struct {
	capturer capture.FrameCapturer
	matcher  *ggvimage.Matcher
	guard    *safety.Guard

	ownsGuard bool
	opts      SessionOptions

	templates map[string]*ggvimage.Template

	// lastFrame retains the previous frame for WaitStable's comparison. Cloned,
	// because the capturer's buffer is overwritten every capture.
	lastFrame *capture.RawFrame

	closed bool
}

// NewSession creates a Session for the given process's main window.
func NewSession(pid uint32, opts *SessionOptions) (*Session, error) {
	capturer, err := capture.NewFrameCapturer(pid)
	if err != nil {
		return nil, utils.WrapError(err, "failed to start capture session")
	}
	return NewSessionWithCapturer(capturer, opts)
}

// NewSessionWithCapturer creates a Session over a caller-supplied frame source.
//
// Useful for driving automation from something other than a live window — a
// recorded frame sequence for regression testing, or a synthetic source in unit
// tests — and for targeting a window found by means other than PID.
//
// The Session takes ownership: Session.Close closes the capturer.
func NewSessionWithCapturer(capturer capture.FrameCapturer, opts *SessionOptions) (*Session, error) {
	if capturer == nil {
		return nil, utils.WrapError(utils.ErrInvalidParameter, "nil frame capturer")
	}
	if opts == nil {
		opts = &SessionOptions{}
	}

	s := &Session{
		capturer:  capturer,
		matcher:   ggvimage.NewMatcher(),
		opts:      *opts,
		templates: make(map[string]*ggvimage.Template),
	}

	switch {
	case opts.DisableGuard:
		// No emergency stop; the caller has asserted the cursor is never moved.
	case opts.Guard != nil:
		s.guard = opts.Guard
	default:
		guard, err := safety.New(nil)
		if err != nil {
			// Failing to arm the emergency stop is not something to paper over:
			// proceeding would leave the user unable to stop a runaway script.
			s.Close()
			return nil, utils.WrapError(err, "failed to arm the emergency stop "+
				"(pass SessionOptions.DisableGuard to run without one)")
		}
		s.guard = guard
		s.ownsGuard = true
	}

	return s, nil
}

// Close releases the session's resources: OpenCV matrices, cached templates, the
// capture session, and the Guard if this Session created it.
func (s *Session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true

	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, tpl := range s.templates {
		record(tpl.Close())
	}
	s.templates = nil

	if s.matcher != nil {
		record(s.matcher.Close())
	}
	if s.capturer != nil {
		record(s.capturer.Close())
	}
	if s.ownsGuard && s.guard != nil {
		record(s.guard.Close())
	}
	s.lastFrame = nil

	return firstErr
}

// Guard returns the session's emergency stop, or nil if disabled.
func (s *Session) Guard() *safety.Guard { return s.guard }

// Window returns the target window's current geometry.
func (s *Session) Window() *capture.WindowInfo { return s.capturer.Window() }

// LoadTemplate decodes a template file and caches it under name.
//
// Call this at startup for every template the script uses. Decoding once and
// keeping the Mat is the difference between a template lookup costing a file
// read plus a decode, and costing nothing.
func (s *Session) LoadTemplate(name, path string) error {
	if _, exists := s.templates[name]; exists {
		return fmt.Errorf("template %q is already registered", name)
	}

	tpl, err := ggvimage.LoadTemplate(name, path)
	if err != nil {
		return err
	}
	s.templates[name] = tpl
	return nil
}

// LoadTemplates loads several templates at once, keyed by name.
//
// Fails on the first error, after closing whatever it had already loaded, so a
// partial load does not leak OpenCV matrices.
func (s *Session) LoadTemplates(paths map[string]string) error {
	loaded := make([]string, 0, len(paths))
	for name, path := range paths {
		if err := s.LoadTemplate(name, path); err != nil {
			for _, n := range loaded {
				if tpl, ok := s.templates[n]; ok {
					tpl.Close()
					delete(s.templates, n)
				}
			}
			return utils.WrapError(err, fmt.Sprintf("failed loading template set at %q", name))
		}
		loaded = append(loaded, name)
	}
	return nil
}

// RegisterTemplate caches an in-memory template under name.
func (s *Session) RegisterTemplate(name string, img stdimage.Image) error {
	if _, exists := s.templates[name]; exists {
		return fmt.Errorf("template %q is already registered", name)
	}

	tpl, err := ggvimage.NewTemplateFromImage(name, img)
	if err != nil {
		return err
	}
	s.templates[name] = tpl
	return nil
}

// Template returns a cached template.
func (s *Session) Template(name string) (*ggvimage.Template, error) {
	tpl, ok := s.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %q is not registered; call LoadTemplate first", name)
	}
	return tpl, nil
}

// Capture grabs the current frame. The returned frame's pixels are only valid
// until the next capture; use RawFrame.Clone to keep one.
func (s *Session) Capture() (*capture.RawFrame, error) {
	if err := s.checkStop(); err != nil {
		return nil, err
	}
	return s.capturer.Capture()
}

// checkStop reports the emergency stop's state, if one is armed.
func (s *Session) checkStop() error {
	if s.guard == nil {
		return nil
	}
	return s.guard.Err()
}

// sleep waits, but stays responsive to the emergency stop.
func (s *Session) sleep(d time.Duration) error {
	if s.guard != nil {
		return s.guard.Sleep(d)
	}
	time.Sleep(d)
	return nil
}

// matchOptions fills in the session's default threshold when the caller left it
// unset.
func (s *Session) matchOptions(opts *ggvimage.MatchOptions) *ggvimage.MatchOptions {
	var out ggvimage.MatchOptions
	if opts != nil {
		out = *opts
	}
	if out.Threshold <= 0 && s.opts.DefaultThreshold > 0 {
		out.Threshold = s.opts.DefaultThreshold
	}
	return &out
}

// Find looks for a registered template in a freshly captured frame.
//
// Returns the match and the frame it was found in; the frame is needed to map the
// match to click coordinates, and returning it avoids a second capture that might
// disagree with the first.
func (s *Session) Find(name string, opts *ggvimage.MatchOptions) (*ggvimage.MatchResult, *capture.RawFrame, error) {
	tpl, err := s.Template(name)
	if err != nil {
		return nil, nil, err
	}

	frame, err := s.Capture()
	if err != nil {
		return nil, nil, err
	}

	match, err := s.matcher.Find(frame, tpl, s.matchOptions(opts))
	if err != nil {
		return nil, frame, err
	}
	return match, frame, nil
}

// Exists reports whether a template is currently visible. A capture or match
// failure is reported as "not visible" together with the error, so callers can
// treat a missing target and a broken capture differently.
func (s *Session) Exists(name string, opts *ggvimage.MatchOptions) (bool, error) {
	_, _, err := s.Find(name, opts)
	switch {
	case err == nil:
		return true, nil
	case utils.IsError(err, ggvimage.ErrNoMatch):
		return false, nil
	default:
		return false, err
	}
}

// WaitOptions configures the Wait* primitives.
type WaitOptions struct {
	// Timeout bounds the wait. Zero means DefaultWaitTimeout.
	//
	// There is deliberately no "wait forever" option: an unbounded wait on a
	// condition that never arrives is indistinguishable from a hung script.
	Timeout time.Duration

	// PollInterval overrides the session's poll interval for this wait.
	PollInterval time.Duration

	// Match configures the underlying template search, including ROI.
	Match *ggvimage.MatchOptions
}

// DefaultWaitTimeout bounds a Wait* call that does not specify a timeout.
const DefaultWaitTimeout = 30 * time.Second

func (o *WaitOptions) timeout() time.Duration {
	if o == nil || o.Timeout <= 0 {
		return DefaultWaitTimeout
	}
	return o.Timeout
}

func (o *WaitOptions) pollInterval(fallback time.Duration) time.Duration {
	if o == nil || o.PollInterval <= 0 {
		return fallback
	}
	return o.PollInterval
}

func (o *WaitOptions) match() *ggvimage.MatchOptions {
	if o == nil {
		return nil
	}
	return o.Match
}

// WaitFor blocks until a template appears, or the timeout expires.
//
// This is the primitive that replaces sleep-and-hope. A fixed time.Sleep after
// an action is either too short — and the script acts on a screen that has not
// finished changing — or too long, and every iteration pays for the worst case.
func (s *Session) WaitFor(name string, opts *WaitOptions) (*ggvimage.MatchResult, *capture.RawFrame, error) {
	tpl, err := s.Template(name)
	if err != nil {
		return nil, nil, err
	}

	deadline := time.Now().Add(opts.timeout())
	interval := opts.pollInterval(s.opts.pollInterval())
	matchOpts := s.matchOptions(opts.match())

	var lastErr error
	for attempt := 0; ; attempt++ {
		if err := s.checkStop(); err != nil {
			return nil, nil, err
		}

		frame, err := s.capturer.Capture()
		if err != nil {
			lastErr = err
		} else {
			match, err := s.matcher.Find(frame, tpl, matchOpts)
			if err == nil {
				utils.Debug("WaitFor(%q) matched after %d attempts (similarity %.3f)",
					name, attempt+1, match.Similarity)
				return match, frame, nil
			}
			lastErr = err
		}

		if remaining := time.Until(deadline); remaining <= 0 {
			return nil, nil, fmt.Errorf("%w: template %q did not appear within %v (last: %v)",
				ErrTimeout, name, opts.timeout(), lastErr)
		} else if remaining < interval {
			// Don't overshoot the deadline by a whole poll interval.
			interval = remaining
		}

		if err := s.sleep(interval); err != nil {
			return nil, nil, err
		}
	}
}

// WaitVanish blocks until a template is no longer visible.
//
// Needed as often as WaitFor: dismissing a dialog, waiting for a loading spinner
// to clear, or confirming an enemy is dead are all "wait for absence".
func (s *Session) WaitVanish(name string, opts *WaitOptions) error {
	tpl, err := s.Template(name)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(opts.timeout())
	interval := opts.pollInterval(s.opts.pollInterval())
	matchOpts := s.matchOptions(opts.match())

	// Track whether we ever managed to look. If every capture failed we time out
	// having learned nothing, and saying "still visible" would send the caller
	// hunting for a template problem that does not exist.
	var lastCaptureErr error
	sawFrame := false

	for attempt := 0; ; attempt++ {
		if err := s.checkStop(); err != nil {
			return err
		}

		frame, err := s.capturer.Capture()
		if err != nil {
			// A capture failure is not evidence of absence; keep trying until the
			// deadline rather than reporting a false success.
			lastCaptureErr = err
			utils.Debug("WaitVanish(%q): capture failed: %v", name, err)
		} else {
			sawFrame = true
			if _, err := s.matcher.Find(frame, tpl, matchOpts); err != nil {
				if utils.IsError(err, ggvimage.ErrNoMatch) {
					utils.Debug("WaitVanish(%q) satisfied after %d attempts", name, attempt+1)
					return nil
				}
				return err
			}
		}

		if remaining := time.Until(deadline); remaining <= 0 {
			if !sawFrame {
				return fmt.Errorf("%w: could not capture the window even once in %v "+
					"(last capture error: %v)", ErrTimeout, opts.timeout(), lastCaptureErr)
			}
			return fmt.Errorf("%w: template %q was still visible after %v",
				ErrTimeout, name, opts.timeout())
		} else if remaining < interval {
			interval = remaining
		}

		if err := s.sleep(interval); err != nil {
			return err
		}
	}
}

// StableOptions configures WaitStable.
type StableOptions struct {
	// Timeout bounds the wait. Zero means DefaultWaitTimeout.
	Timeout time.Duration

	// PollInterval is the gap between compared frames. Zero uses the session's.
	PollInterval time.Duration

	// ROI restricts comparison to a region. Zero value compares the whole frame.
	//
	// Worth setting: a clock, a chat feed or an idle animation elsewhere on screen
	// will keep the full frame permanently "unstable".
	ROI stdimage.Rectangle

	// Tolerance is the fraction of sampled pixels allowed to differ while still
	// counting as stable, 0-1. Zero means DefaultStableTolerance.
	Tolerance float64

	// ChannelDelta is how much a single channel may differ before that pixel
	// counts as changed, 0-255. Zero means DefaultChannelDelta, which absorbs
	// video compression and dithering noise without missing real changes.
	ChannelDelta int

	// RequiredStreak is how many consecutive stable comparisons are needed. Zero
	// means DefaultStableStreak.
	RequiredStreak int
}

// Defaults for StableOptions.
const (
	DefaultStableTolerance = 0.005 // 0.5% of sampled pixels
	DefaultChannelDelta    = 8
	DefaultStableStreak    = 2
)

func (o *StableOptions) timeout() time.Duration {
	if o == nil || o.Timeout <= 0 {
		return DefaultWaitTimeout
	}
	return o.Timeout
}

func (o *StableOptions) tolerance() float64 {
	if o == nil || o.Tolerance <= 0 {
		return DefaultStableTolerance
	}
	return o.Tolerance
}

func (o *StableOptions) channelDelta() int {
	if o == nil || o.ChannelDelta <= 0 {
		return DefaultChannelDelta
	}
	return o.ChannelDelta
}

func (o *StableOptions) streak() int {
	if o == nil || o.RequiredStreak <= 0 {
		return DefaultStableStreak
	}
	return o.RequiredStreak
}

// WaitStable blocks until consecutive frames stop differing — the standard way to
// detect "loading finished" without a template for the loaded state.
//
// Returns the last frame once stable.
func (s *Session) WaitStable(opts *StableOptions) (*capture.RawFrame, error) {
	deadline := time.Now().Add(opts.timeout())

	interval := s.opts.pollInterval()
	if opts != nil && opts.PollInterval > 0 {
		interval = opts.PollInterval
	}

	var roi stdimage.Rectangle
	if opts != nil {
		roi = opts.ROI
	}
	tolerance := opts.tolerance()
	delta := opts.channelDelta()
	required := opts.streak()

	var previous *capture.RawFrame
	streak := 0

	for {
		if err := s.checkStop(); err != nil {
			return nil, err
		}

		frame, err := s.capturer.Capture()
		if err != nil {
			return nil, err
		}

		if previous != nil {
			diff, err := FrameDifference(previous, frame, roi, delta)
			if err != nil {
				return nil, err
			}
			if diff <= tolerance {
				streak++
				if streak >= required {
					utils.Debug("WaitStable satisfied: %d consecutive frames within %.4f (last diff %.4f)",
						streak, tolerance, diff)
					return frame, nil
				}
			} else {
				streak = 0
			}
		}

		// Clone: the capturer overwrites its buffer on the next capture, so
		// holding the frame itself would compare a frame against itself.
		previous = frame.Clone()

		if remaining := time.Until(deadline); remaining <= 0 {
			return nil, fmt.Errorf("%w: frames did not stabilise within %v",
				ErrTimeout, opts.timeout())
		} else if remaining < interval {
			interval = remaining
		}

		if err := s.sleep(interval); err != nil {
			return nil, err
		}
	}
}

// Click clicks a point given in frame pixel coordinates, converting to click
// coordinates using the frame's own window snapshot.
//
// Taking pixels rather than screen coordinates is deliberate: everything a script
// computes from a match is in frame pixels, and making the caller convert is how
// scale-factor mistakes get made.
func (s *Session) Click(frame *capture.RawFrame, px, py int, button mouse.MouseButton) error {
	if err := s.checkStop(); err != nil {
		return err
	}
	if frame == nil {
		return utils.WrapError(utils.ErrInvalidParameter, "nil frame")
	}

	pt := frame.Window.PixelToClickPoint(px, py)
	if s.guard != nil {
		return s.guard.Click(pt.X, pt.Y, button)
	}
	return mouse.BackgroundClick(pt.X, pt.Y, button)
}

// ClickMatch clicks the centre of a match.
//
// The centre, not the top-left corner: a template's corner often sits on a
// transparent or borderline pixel, and clicking it can miss the actual control.
func (s *Session) ClickMatch(frame *capture.RawFrame, match *ggvimage.MatchResult, button mouse.MouseButton) error {
	if match == nil {
		return utils.WrapError(utils.ErrInvalidParameter, "nil match")
	}
	c := match.CenterInPixels()
	return s.Click(frame, c.X, c.Y, button)
}

// FindAndClick locates a template and clicks its centre in one step.
func (s *Session) FindAndClick(name string, opts *ggvimage.MatchOptions, button mouse.MouseButton) (*ggvimage.MatchResult, error) {
	match, frame, err := s.Find(name, opts)
	if err != nil {
		return nil, err
	}
	if err := s.ClickMatch(frame, match, button); err != nil {
		return match, err
	}
	return match, nil
}

// WaitAndClick waits for a template to appear and then clicks its centre.
//
// The most common action in an automation script, and the one most often written
// wrongly as sleep-then-click.
func (s *Session) WaitAndClick(name string, opts *WaitOptions, button mouse.MouseButton) (*ggvimage.MatchResult, error) {
	match, frame, err := s.WaitFor(name, opts)
	if err != nil {
		return nil, err
	}
	if err := s.ClickMatch(frame, match, button); err != nil {
		return match, err
	}
	return match, nil
}

// NewSessionForWindow creates a Session bound to a specific window handle.
//
// Use with capture.FindWindow when the window cannot be reached from a process
// name — most importantly a Wine/CrossOver game, where the process is
// `wine64-preloader` and only the window title identifies the game.
func NewSessionForWindow(handle uintptr, opts *SessionOptions) (*Session, error) {
	capturer, err := capture.NewFrameCapturerForWindow(handle)
	if err != nil {
		return nil, utils.WrapError(err, "failed to start capture session for window")
	}
	return NewSessionWithCapturer(capturer, opts)
}

// NewSessionByTitle finds a window by title substring and opens a Session on it.
//
// The recommended entry point on macOS: the title is set by the game itself and
// survives Wine wrappers, Steam launchers and renamed binaries, none of which a
// PID lookup handles.
func NewSessionByTitle(titleSubstring string, opts *SessionOptions) (*Session, error) {
	win, err := capture.FindWindowByTitle(titleSubstring)
	if err != nil {
		return nil, utils.WrapError(err, fmt.Sprintf(
			"no window found with a title containing %q", titleSubstring))
	}
	utils.Info("matched window: %s", win)
	return NewSessionForWindow(win.Handle, opts)
}
