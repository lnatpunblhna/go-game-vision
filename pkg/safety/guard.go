// Package safety provides the emergency stop for foreground automation.
//
// This is not optional polish. On macOS there is no true background click: the
// only available mechanism, CGEventPost, drives the real system cursor, so a
// running script owns the mouse and the user cannot reach a stop button. A
// script with a bad template or an unexpected dialog will happily click forever
// on a machine its owner can no longer control. The same applies on Windows
// whenever SendInput is used instead of PostMessage.
//
// A Guard watches for an abort gesture on a background goroutine and cancels a
// context when it sees one. Automation loops check that context between actions,
// and the wrapper methods here (Click, Sleep) check it for you.
package safety

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Corner identifies a screen corner used as an abort gesture.
type Corner int

const (
	TopLeft Corner = iota
	TopRight
	BottomLeft
	BottomRight
)

// String returns a readable corner name.
func (c Corner) String() string {
	switch c {
	case TopLeft:
		return "top-left"
	case TopRight:
		return "top-right"
	case BottomLeft:
		return "bottom-left"
	case BottomRight:
		return "bottom-right"
	default:
		return "unknown"
	}
}

// Options configures a Guard. A nil *Options uses defaults throughout.
type Options struct {
	// Corners lists the screen corners that abort when the cursor rests in one.
	// Empty means {TopLeft}.
	//
	// TopLeft is the default because it is the one corner that exists at (0,0) on
	// every display configuration; the others depend on resolution, and on a
	// multi-monitor setup the primary screen's right edge is in the middle of the
	// desktop rather than at its border.
	Corners []Corner

	// CornerMargin is how close to a corner, in pixels, counts as being in it.
	// Zero means DefaultCornerMargin.
	//
	// Keep it small. Automation that clicks near a corner moves the real cursor
	// there on macOS, and too generous a margin turns the script's own clicks
	// into spurious aborts.
	CornerMargin int

	// PollInterval is how often the cursor is sampled. Zero means
	// DefaultPollInterval.
	PollInterval time.Duration

	// ConsecutiveSamples is how many successive in-corner samples are required
	// before aborting. Zero means DefaultConsecutiveSamples.
	//
	// Requiring more than one is what distinguishes a deliberate "park the mouse
	// in the corner" gesture from the cursor merely passing through, including
	// when the script itself put it there.
	ConsecutiveSamples int

	// CatchSignals aborts on SIGINT/SIGTERM as well, so Ctrl-C stops the
	// automation cleanly instead of killing the process mid-click. Defaults to
	// true; set DisableSignals to opt out.
	DisableSignals bool

	// Custom is an optional extra abort condition, polled on the same interval.
	// Returning true aborts with the given reason. Must not block.
	Custom func() (bool, string)
}

// Defaults for Options.
const (
	DefaultCornerMargin       = 3
	DefaultPollInterval       = 50 * time.Millisecond
	DefaultConsecutiveSamples = 2
)

func (o *Options) corners() []Corner {
	if o == nil || len(o.Corners) == 0 {
		return []Corner{TopLeft}
	}
	return o.Corners
}

func (o *Options) margin() int {
	if o == nil || o.CornerMargin <= 0 {
		return DefaultCornerMargin
	}
	return o.CornerMargin
}

func (o *Options) pollInterval() time.Duration {
	if o == nil || o.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return o.PollInterval
}

func (o *Options) consecutive() int {
	if o == nil || o.ConsecutiveSamples <= 0 {
		return DefaultConsecutiveSamples
	}
	return o.ConsecutiveSamples
}

// Guard watches for an abort gesture and cancels its context when one occurs.
//
// Safe for concurrent use.
type Guard struct {
	ctx    context.Context
	cancel context.CancelFunc

	opts    Options
	screenW int
	screenH int

	reason atomic.Value // string

	stopWatch chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup

	signalCh chan os.Signal
}

// New creates a Guard and starts watching.
//
// Returns an error if the screen size cannot be read, since corner detection
// depends on it. Call Close when the automation finishes.
func New(opts *Options) (*Guard, error) {
	if opts == nil {
		opts = &Options{}
	}

	clicker := mouse.NewMouseClicker()
	w, h, err := clicker.GetScreenSize()
	if err != nil {
		return nil, utils.WrapError(err, "emergency stop needs the screen size for corner detection")
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := &Guard{
		ctx:       ctx,
		cancel:    cancel,
		opts:      *opts,
		screenW:   w,
		screenH:   h,
		stopWatch: make(chan struct{}),
	}
	g.reason.Store("")

	g.wg.Add(1)
	go g.watch()

	if !opts.DisableSignals {
		g.signalCh = make(chan os.Signal, 1)
		signal.Notify(g.signalCh, os.Interrupt, syscall.SIGTERM)
		g.wg.Add(1)
		go g.watchSignals()
	}

	corners := make([]string, 0, len(opts.corners()))
	for _, c := range opts.corners() {
		corners = append(corners, c.String())
	}
	utils.Info("emergency stop armed: park the cursor in %v (within %dpx) to abort",
		corners, opts.margin())

	return g, nil
}

// Context returns the context cancelled when the abort gesture fires. Pass it
// into anything that should stop promptly.
func (g *Guard) Context() context.Context { return g.ctx }

// Done returns a channel closed when the Guard trips.
func (g *Guard) Done() <-chan struct{} { return g.ctx.Done() }

// Err returns a non-nil error once the Guard has tripped. Check this between
// actions in a loop.
func (g *Guard) Err() error {
	if err := g.ctx.Err(); err != nil {
		return fmt.Errorf("automation aborted (%s): %w", g.Reason(), err)
	}
	return nil
}

// Tripped reports whether the abort gesture has fired.
func (g *Guard) Tripped() bool { return g.ctx.Err() != nil }

// Reason describes what caused the abort, or "" if it has not fired.
func (g *Guard) Reason() string {
	if s, ok := g.reason.Load().(string); ok {
		return s
	}
	return ""
}

// Trip aborts immediately with the given reason. Idempotent; the first reason
// wins, so the original cause is not overwritten by later trips.
func (g *Guard) Trip(reason string) {
	if g.ctx.Err() != nil {
		return
	}
	g.reason.CompareAndSwap("", reason)
	utils.Warn("emergency stop triggered: %s", reason)
	g.cancel()
}

// Close stops watching and releases resources. Does not itself trip the Guard.
func (g *Guard) Close() error {
	g.closeOnce.Do(func() {
		close(g.stopWatch)
		if g.signalCh != nil {
			signal.Stop(g.signalCh)
		}
		// Unblock the signal goroutine, which is parked on a receive.
		g.cancel()
		g.wg.Wait()
	})
	return nil
}

// watch polls the cursor position for the abort gesture.
func (g *Guard) watch() {
	defer g.wg.Done()

	ticker := time.NewTicker(g.opts.pollInterval())
	defer ticker.Stop()

	required := g.opts.consecutive()
	streak := 0
	var streakCorner Corner

	for {
		select {
		case <-g.stopWatch:
			return
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}

		if g.opts.Custom != nil {
			if stop, reason := g.opts.Custom(); stop {
				if reason == "" {
					reason = "custom stop condition"
				}
				g.Trip(reason)
				return
			}
		}

		x, y, err := mouse.GetCursorPosition()
		if err != nil {
			// Don't abort on a transient read failure: killing a working script
			// because one cursor query failed is worse than missing one sample.
			utils.Debug("emergency stop: could not read cursor position: %v", err)
			streak = 0
			continue
		}

		corner, inCorner := g.cornerAt(x, y)
		switch {
		case !inCorner:
			streak = 0
		case streak > 0 && corner != streakCorner:
			// Moved to a different corner; restart the count for that one.
			streak = 1
			streakCorner = corner
		default:
			streak++
			streakCorner = corner
		}

		if inCorner && streak >= required {
			g.Trip(fmt.Sprintf("cursor parked in %s corner at (%d,%d)", corner, x, y))
			return
		}
	}
}

// watchSignals aborts on SIGINT/SIGTERM.
func (g *Guard) watchSignals() {
	defer g.wg.Done()
	select {
	case sig, ok := <-g.signalCh:
		if !ok {
			return
		}
		g.Trip(fmt.Sprintf("received signal %s", sig))
	case <-g.ctx.Done():
	}
}

// cornerAt reports which configured corner, if any, contains the point.
func (g *Guard) cornerAt(x, y int) (Corner, bool) {
	m := g.opts.margin()

	for _, c := range g.opts.corners() {
		var cx, cy int
		switch c {
		case TopLeft:
			cx, cy = 0, 0
		case TopRight:
			cx, cy = g.screenW-1, 0
		case BottomLeft:
			cx, cy = 0, g.screenH-1
		case BottomRight:
			cx, cy = g.screenW-1, g.screenH-1
		default:
			continue
		}

		if absInt(x-cx) <= m && absInt(y-cy) <= m {
			return c, true
		}
	}
	return TopLeft, false
}

// Click performs a background click, refusing to act once the Guard has tripped.
//
// Routing clicks through the Guard is what makes the emergency stop effective:
// checking the context only at the top of a loop still lets a long action
// sequence run to completion after the user has asked it to stop.
func (g *Guard) Click(x, y int, button mouse.MouseButton) error {
	if err := g.Err(); err != nil {
		return err
	}
	return mouse.BackgroundClick(x, y, button)
}

// ClickWithOptions is Click with explicit click options.
func (g *Guard) ClickWithOptions(x, y int, options *mouse.ClickOptions) error {
	if err := g.Err(); err != nil {
		return err
	}
	return mouse.NewMouseClicker().BackgroundClick(x, y, options)
}

// Sleep waits for d, returning early if the Guard trips.
//
// Prefer this over time.Sleep in automation code: a plain sleep cannot be
// interrupted, so a script pausing between actions stays unresponsive to the
// abort gesture for the whole duration.
func (g *Guard) Sleep(d time.Duration) error {
	if err := g.Err(); err != nil {
		return err
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-g.ctx.Done():
		return g.Err()
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
