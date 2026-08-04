package automation

import (
	"errors"
	"fmt"
	stdimage "image"
	"sort"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// The decision layer.
//
// Everything below this file is perception: capture a frame, find a thing in it,
// click a point. None of it decides *what to do*, and a script written directly
// against it becomes a tree of nested ifs that is impossible to extend — each new
// case has to be threaded through the existing control flow by hand.
//
// A Machine inverts that. A state is a recognisable screen (a menu, a loading
// screen, a battle, a reward popup) paired with what to do when you are on it. The
// loop is fixed — recognise, look up, act, re-recognise — so adding behaviour means
// adding a state, not editing control flow.
//
// The part that is easy to skip and shouldn't be is the fallback. Real games throw
// screens you did not plan for: a network error, a daily-login popup, an update
// prompt. Without an explicit "I don't recognise this" path, the loop spins
// forever on an unknown screen and the script appears to hang. MachineOptions
// makes that path mandatory to think about.

// Done is returned from a State's Action to stop the machine successfully.
var Done = errors.New("automation complete")

// ErrUnknownState is returned when the machine cannot recognise the screen for
// too many consecutive iterations and no recovery handler resolved it.
var ErrUnknownState = errors.New("could not recognise the current screen")

// ErrStuck is returned when the machine stays in one state for too many
// consecutive iterations, which normally means its Action is not having the
// effect it assumes.
var ErrStuck = errors.New("state machine appears stuck")

// Context is passed to a state's Detect and Action callbacks.
//
// It carries the frame captured for the current iteration. Detect callbacks share
// that single frame deliberately: capturing per candidate state would multiply the
// dominant cost of the loop by the number of states, and would also let different
// states make decisions about different moments in time.
type Context struct {
	// Session is the underlying automation session.
	Session *Session

	// Frame is the frame captured for this iteration. Its pixels are only valid
	// for the duration of the callback.
	Frame *capture.RawFrame

	// Iteration counts loop passes, starting at 0.
	Iteration int

	// State is the name of the state being acted on. Empty inside Detect.
	State string

	// PreviousState is the state acted on last iteration, or "" on the first.
	PreviousState string

	// StateStreak is how many consecutive iterations have been spent in State.
	StateStreak int

	// Elapsed is how long the machine has been running.
	Elapsed time.Duration
}

// Find searches the current frame for a registered template.
//
// Uses the frame already captured for this iteration rather than taking a new one,
// so every decision in an iteration is made about the same moment.
func (c *Context) Find(name string, opts *ggvimage.MatchOptions) (*ggvimage.MatchResult, error) {
	tpl, err := c.Session.Template(name)
	if err != nil {
		return nil, err
	}
	return c.Session.matcher.Find(c.Frame, tpl, c.Session.matchOptions(opts))
}

// Visible reports whether a template is present in the current frame.
func (c *Context) Visible(name string, opts *ggvimage.MatchOptions) (bool, error) {
	_, err := c.Find(name, opts)
	switch {
	case err == nil:
		return true, nil
	case utils.IsError(err, ggvimage.ErrNoMatch):
		return false, nil
	default:
		return false, err
	}
}

// Click clicks a point in current-frame pixel coordinates.
func (c *Context) Click(px, py int, button mouse.MouseButton) error {
	return c.Session.Click(c.Frame, px, py, button)
}

// ClickTemplate finds a template in the current frame and clicks its centre.
func (c *Context) ClickTemplate(name string, opts *ggvimage.MatchOptions) error {
	match, err := c.Find(name, opts)
	if err != nil {
		return err
	}
	return c.Session.ClickMatch(c.Frame, match, mouse.LeftButton)
}

// Sleep waits, staying responsive to the emergency stop.
func (c *Context) Sleep(d time.Duration) error {
	return c.Session.sleep(d)
}

// State is one recognisable screen and what to do about it.
type State struct {
	// Name identifies the state. Must be unique and non-empty.
	Name string

	// Detect reports whether the current frame shows this state.
	//
	// Should be cheap and side-effect free: it is called on every iteration until
	// it matches. Prefer a narrow ROI — see TemplateVisible.
	Detect func(*Context) (bool, error)

	// Action does the work for this state.
	//
	// Return Done to finish the run successfully. Return a non-empty string to
	// declare which state you expect next; the machine verifies that expectation
	// and reports a clear error if it is wrong, which turns a silent
	// misunderstanding into a diagnosable one. Return "" to simply re-recognise.
	Action func(*Context) (string, error)

	// Priority orders detection, highest first. States with equal priority are
	// ordered by name for determinism.
	//
	// Matters when states overlap: a modal error dialog drawn over the battle
	// screen means both detectors match, and the dialog must win or the script
	// will keep trying to fight through it. Give interrupting states a high
	// priority.
	Priority int

	// Terminal marks a state whose mere detection ends the run successfully,
	// without needing an Action.
	Terminal bool
}

// TemplateVisible builds a Detect function that matches when a template is
// present.
//
// The common case, and worth using with an explicit ROI in opts: a state detector
// runs every iteration, so a full-frame search here is the easiest way to turn a
// 25 fps loop into a 6 fps one.
func TemplateVisible(name string, opts *ggvimage.MatchOptions) func(*Context) (bool, error) {
	return func(c *Context) (bool, error) {
		return c.Visible(name, opts)
	}
}

// TemplateAbsent builds a Detect function that matches when a template is gone.
func TemplateAbsent(name string, opts *ggvimage.MatchOptions) func(*Context) (bool, error) {
	return func(c *Context) (bool, error) {
		visible, err := c.Visible(name, opts)
		if err != nil {
			return false, err
		}
		return !visible, nil
	}
}

// AllVisible builds a Detect function requiring every named template to be
// present — for screens that only one combination of elements identifies
// uniquely.
func AllVisible(names []string, opts *ggvimage.MatchOptions) func(*Context) (bool, error) {
	return func(c *Context) (bool, error) {
		for _, name := range names {
			visible, err := c.Visible(name, opts)
			if err != nil {
				return false, err
			}
			if !visible {
				return false, nil
			}
		}
		return len(names) > 0, nil
	}
}

// AnyVisible builds a Detect function matching when at least one named template
// is present.
func AnyVisible(names []string, opts *ggvimage.MatchOptions) func(*Context) (bool, error) {
	return func(c *Context) (bool, error) {
		for _, name := range names {
			visible, err := c.Visible(name, opts)
			if err != nil {
				return false, err
			}
			if visible {
				return true, nil
			}
		}
		return false, nil
	}
}

// MachineOptions configures a Machine.
type MachineOptions struct {
	// Timeout bounds a whole run. Zero means DefaultRunTimeout.
	Timeout time.Duration

	// IterationDelay is the pause between loop passes. Zero means
	// DefaultIterationDelay.
	//
	// Not just politeness: without a pause the loop captures as fast as the
	// machine allows, burning CPU to re-examine a screen that has not had time to
	// change in response to the last action.
	IterationDelay time.Duration

	// MaxUnknownStreak is how many consecutive unrecognised frames to tolerate
	// before giving up (or invoking OnUnknown). Zero means
	// DefaultMaxUnknownStreak.
	MaxUnknownStreak int

	// OnUnknown is called when the screen is unrecognised, before the streak
	// limit is reached.
	//
	// This is the escape hatch for the popup you did not anticipate: a typical
	// implementation presses Escape or clicks a known "close" button to get back
	// to familiar ground. Returning nil means "I tried something, carry on";
	// returning an error aborts the run.
	OnUnknown func(*Context) error

	// MaxStateStreak is how many consecutive iterations one state may persist
	// before it is treated as stuck. Zero means DefaultMaxStateStreak.
	MaxStateStreak int

	// OnStuck is called when a state exceeds MaxStateStreak. Returning nil resets
	// the streak and continues; returning an error aborts.
	OnStuck func(*Context) error

	// OnTransition is called after each successful state action, for logging or
	// progress reporting.
	OnTransition func(from, to string, iteration int)
}

// Defaults for MachineOptions.
const (
	DefaultRunTimeout       = 10 * time.Minute
	DefaultIterationDelay   = 200 * time.Millisecond
	DefaultMaxUnknownStreak = 10
	DefaultMaxStateStreak   = 30
)

func (o *MachineOptions) timeout() time.Duration {
	if o == nil || o.Timeout <= 0 {
		return DefaultRunTimeout
	}
	return o.Timeout
}

func (o *MachineOptions) iterationDelay() time.Duration {
	if o == nil || o.IterationDelay <= 0 {
		return DefaultIterationDelay
	}
	return o.IterationDelay
}

func (o *MachineOptions) maxUnknownStreak() int {
	if o == nil || o.MaxUnknownStreak <= 0 {
		return DefaultMaxUnknownStreak
	}
	return o.MaxUnknownStreak
}

func (o *MachineOptions) maxStateStreak() int {
	if o == nil || o.MaxStateStreak <= 0 {
		return DefaultMaxStateStreak
	}
	return o.MaxStateStreak
}

// Machine runs the recognise-decide-act loop over a set of states.
//
// Not safe for concurrent use.
type Machine struct {
	session *Session
	opts    MachineOptions

	states map[string]*State
	order  []*State // sorted by descending priority, then name

	// stats
	visits map[string]int
}

// NewMachine creates a state machine driving the given session.
func NewMachine(session *Session, opts *MachineOptions) *Machine {
	if opts == nil {
		opts = &MachineOptions{}
	}
	return &Machine{
		session: session,
		opts:    *opts,
		states:  make(map[string]*State),
		visits:  make(map[string]int),
	}
}

// AddState registers a state.
func (m *Machine) AddState(st *State) error {
	if st == nil {
		return utils.WrapError(utils.ErrInvalidParameter, "nil state")
	}
	if st.Name == "" {
		return utils.WrapError(utils.ErrInvalidParameter, "state name must not be empty")
	}
	if st.Detect == nil {
		return utils.WrapError(utils.ErrInvalidParameter,
			fmt.Sprintf("state %q has no Detect function", st.Name))
	}
	if st.Action == nil && !st.Terminal {
		// A non-terminal state without an action would be detected forever with
		// nothing to advance it — the definition of a stuck machine.
		return utils.WrapError(utils.ErrInvalidParameter,
			fmt.Sprintf("state %q needs an Action, or must be marked Terminal", st.Name))
	}
	if _, exists := m.states[st.Name]; exists {
		return fmt.Errorf("state %q is already registered", st.Name)
	}

	copied := *st
	m.states[st.Name] = &copied
	m.order = append(m.order, &copied)

	// Highest priority first; ties broken by name so detection order is stable
	// rather than dependent on registration order.
	sort.SliceStable(m.order, func(i, j int) bool {
		if m.order[i].Priority != m.order[j].Priority {
			return m.order[i].Priority > m.order[j].Priority
		}
		return m.order[i].Name < m.order[j].Name
	})
	return nil
}

// AddStates registers several states, stopping at the first error.
func (m *Machine) AddStates(states ...*State) error {
	for _, st := range states {
		if err := m.AddState(st); err != nil {
			return err
		}
	}
	return nil
}

// Visits reports how many times each state was acted on during the last run.
// Useful for spotting a state that is being entered far more often than the
// script's logic implies.
func (m *Machine) Visits() map[string]int {
	out := make(map[string]int, len(m.visits))
	for k, v := range m.visits {
		out[k] = v
	}
	return out
}

// Detect identifies the current screen without acting on it.
//
// Returns the matching state's name, or "" if nothing matched.
func (m *Machine) Detect() (string, *capture.RawFrame, error) {
	frame, err := m.session.Capture()
	if err != nil {
		return "", nil, err
	}

	ctx := &Context{Session: m.session, Frame: frame}
	name, err := m.detectIn(ctx)
	return name, frame, err
}

// detectIn finds the highest-priority state matching the context's frame.
func (m *Machine) detectIn(ctx *Context) (string, error) {
	for _, st := range m.order {
		matched, err := st.Detect(ctx)
		if err != nil {
			// A detector that errors is a bug in the script (usually an
			// unregistered template), not a "screen not recognised" condition, so
			// surface it rather than silently treating the state as absent.
			return "", utils.WrapError(err, fmt.Sprintf("detector for state %q failed", st.Name))
		}
		if matched {
			return st.Name, nil
		}
	}
	return "", nil
}

// Run drives the loop until a terminal state, Done, the timeout, or an error.
func (m *Machine) Run() error {
	if len(m.order) == 0 {
		return utils.WrapError(utils.ErrInvalidParameter, "no states registered")
	}

	m.visits = make(map[string]int)

	start := time.Now()
	deadline := start.Add(m.opts.timeout())

	var previous string
	var stateStreak int
	unknownStreak := 0
	var expected string

	for iteration := 0; ; iteration++ {
		if err := m.session.checkStop(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: state machine ran for %v without completing (last state %q)",
				ErrTimeout, m.opts.timeout(), previous)
		}

		frame, err := m.session.capturer.Capture()
		if err != nil {
			// Transient capture failures are normal — a window being resized, a
			// display sleeping. Count them towards the unknown streak so a
			// permanently broken capture still terminates.
			unknownStreak++
			utils.Debug("machine iteration %d: capture failed: %v", iteration, err)
			if unknownStreak >= m.opts.maxUnknownStreak() {
				return utils.WrapError(err, fmt.Sprintf(
					"capture failed %d times in a row", unknownStreak))
			}
			if err := m.session.sleep(m.opts.iterationDelay()); err != nil {
				return err
			}
			continue
		}

		ctx := &Context{
			Session:       m.session,
			Frame:         frame,
			Iteration:     iteration,
			PreviousState: previous,
			Elapsed:       time.Since(start),
		}

		name, err := m.detectIn(ctx)
		if err != nil {
			return err
		}

		if name == "" {
			unknownStreak++
			utils.Warn("machine iteration %d: screen not recognised (streak %d/%d)",
				iteration, unknownStreak, m.opts.maxUnknownStreak())

			if unknownStreak >= m.opts.maxUnknownStreak() {
				return fmt.Errorf("%w: %d consecutive unrecognised frames (last known state %q)",
					ErrUnknownState, unknownStreak, previous)
			}
			if m.opts.OnUnknown != nil {
				if err := m.opts.OnUnknown(ctx); err != nil {
					return utils.WrapError(err, "unknown-state handler failed")
				}
			}
			if err := m.session.sleep(m.opts.iterationDelay()); err != nil {
				return err
			}
			continue
		}
		unknownStreak = 0

		// If the previous action declared what should come next, verify it. A
		// mismatch usually means the action did not do what its author believed,
		// and saying so beats looping until the overall timeout.
		if expected != "" && name != expected {
			return fmt.Errorf("state %q expected %q next, but the screen is %q",
				previous, expected, name)
		}
		expected = ""

		if name == previous {
			stateStreak++
		} else {
			stateStreak = 1
		}
		ctx.State = name
		ctx.StateStreak = stateStreak

		if stateStreak > m.opts.maxStateStreak() {
			if m.opts.OnStuck == nil {
				return fmt.Errorf("%w: state %q persisted for %d iterations",
					ErrStuck, name, stateStreak)
			}
			utils.Warn("state %q stuck for %d iterations, invoking recovery", name, stateStreak)
			if err := m.opts.OnStuck(ctx); err != nil {
				return utils.WrapError(err, fmt.Sprintf("stuck handler for state %q failed", name))
			}
			stateStreak = 0
			if err := m.session.sleep(m.opts.iterationDelay()); err != nil {
				return err
			}
			continue
		}

		st := m.states[name]
		m.visits[name]++

		if st.Terminal {
			utils.Info("machine reached terminal state %q after %d iterations (%v)",
				name, iteration+1, time.Since(start).Round(time.Millisecond))
			if m.opts.OnTransition != nil {
				m.opts.OnTransition(previous, name, iteration)
			}
			return nil
		}

		next, err := st.Action(ctx)
		switch {
		case errors.Is(err, Done):
			utils.Info("machine finished at state %q after %d iterations (%v)",
				name, iteration+1, time.Since(start).Round(time.Millisecond))
			if m.opts.OnTransition != nil {
				m.opts.OnTransition(previous, name, iteration)
			}
			return nil
		case err != nil:
			return utils.WrapError(err, fmt.Sprintf("action for state %q failed", name))
		}

		if next != "" {
			if _, ok := m.states[next]; !ok {
				return fmt.Errorf("state %q declared next state %q, which is not registered",
					name, next)
			}
			expected = next
		}

		if m.opts.OnTransition != nil {
			m.opts.OnTransition(previous, name, iteration)
		}
		previous = name

		if err := m.session.sleep(m.opts.iterationDelay()); err != nil {
			return err
		}
	}
}

// ClickAndExpect is a helper for the most common Action body: click a template,
// then declare which state should follow.
func ClickAndExpect(template, nextState string, opts *ggvimage.MatchOptions) func(*Context) (string, error) {
	return func(c *Context) (string, error) {
		if err := c.ClickTemplate(template, opts); err != nil {
			return "", err
		}
		return nextState, nil
	}
}

// ClickTemplateAction is an Action that clicks a template and re-recognises.
func ClickTemplateAction(template string, opts *ggvimage.MatchOptions) func(*Context) (string, error) {
	return func(c *Context) (string, error) {
		return "", c.ClickTemplate(template, opts)
	}
}

// ClickPointAction is an Action that clicks a fixed point in frame pixels.
//
// Use sparingly: a hard-coded point breaks the moment the window is resized or
// the UI moves, whereas a template match follows it.
func ClickPointAction(px, py int) func(*Context) (string, error) {
	return func(c *Context) (string, error) {
		return "", c.Click(px, py, mouse.LeftButton)
	}
}

// RegionOf is a small helper for building ROIs from a fraction of the frame,
// so state detectors can be written without hard-coding pixel geometry for one
// particular resolution.
func RegionOf(frame *capture.RawFrame, xFrac, yFrac, wFrac, hFrac float64) stdimage.Rectangle {
	if frame == nil {
		return stdimage.Rectangle{}
	}
	x := int(float64(frame.Width) * xFrac)
	y := int(float64(frame.Height) * yFrac)
	w := int(float64(frame.Width) * wFrac)
	h := int(float64(frame.Height) * hFrac)
	return stdimage.Rect(x, y, x+w, y+h)
}
