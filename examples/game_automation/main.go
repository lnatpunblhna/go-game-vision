// End-to-end example of the automation stack: find a window, load templates,
// define states, and let the machine drive.
//
// Run with the window title to target:
//
//	go run ./examples/game_automation -title "Stardew Valley"
//
// Targeting by title rather than by process name is deliberate — see the comment
// on findTarget.
package main

import (
	"flag"
	"fmt"
	stdimage "image"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/automation"
	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/keyboard"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/safety"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func main() {
	title := flag.String("title", "", "window title substring to target")
	list := flag.Bool("list", false, "list all windows and exit")
	templateDir := flag.String("templates", "", "directory of PNG templates to load")
	flag.Parse()

	utils.SetLogLevel(utils.INFO)

	if *list {
		listWindows()
		return
	}
	if *title == "" {
		fmt.Fprintln(os.Stderr, "need -title (or -list to see what is available)")
		os.Exit(2)
	}

	if err := run(*title, *templateDir); err != nil {
		log.Fatalf("automation failed: %v", err)
	}
}

// listWindows prints what the window server can see.
//
// The first thing to run when a target cannot be found: it shows the real title
// and owner of every window, which is how you discover that a Wine game's process
// is called `wine64-preloader` while its window is called something sensible.
func listWindows() {
	windows, err := capture.ListWindows()
	if err != nil {
		log.Fatalf("could not list windows: %v", err)
	}
	fmt.Printf("%d windows (largest first):\n", len(windows))
	for _, w := range windows {
		fmt.Printf("  %s\n", w)
	}
}

func run(title, templateDir string) error {
	// The emergency stop, created explicitly so its configuration is visible.
	// Without one, a script that drives the real cursor leaves no way to regain
	// control of the machine.
	guard, err := safety.New(&safety.Options{
		Corners:      []safety.Corner{safety.TopLeft, safety.TopRight},
		CornerMargin: 4,
	})
	if err != nil {
		return fmt.Errorf("could not arm the emergency stop: %w", err)
	}
	defer guard.Close()
	fmt.Println("Emergency stop: park the cursor in a top corner, or press Ctrl-C.")

	// Title-first lookup. On macOS this is the reliable path: the title comes from
	// the game, while the process name may be a Wine wrapper or a launcher.
	session, err := automation.NewSessionByTitle(title, &automation.SessionOptions{
		Guard:            guard,
		DefaultThreshold: 0.85,
		PollInterval:     120 * time.Millisecond,
	})
	if err != nil {
		return fmt.Errorf("could not open a session for %q: %w", title, err)
	}
	defer session.Close()

	win := session.Window()
	fmt.Printf("Attached to %q: %dx%d at (%d,%d), scale %.1fx\n",
		win.Title, win.Rect.Dx(), win.Rect.Dy(), win.Rect.Min.X, win.Rect.Min.Y,
		win.ScaleOrDefault())

	if templateDir != "" {
		if err := loadTemplates(session, templateDir); err != nil {
			return err
		}
	}

	// Demonstrate the perception layer against the live window.
	if err := reportFrame(session); err != nil {
		return err
	}

	// Then the decision layer.
	return runMachine(session)
}

// loadTemplates loads every PNG in a directory, keyed by filename without
// extension. Loading happens once here rather than per frame: a Template holds a
// decoded OpenCV matrix, and re-decoding it every iteration is the single easiest
// way to make a capture loop slow.
func loadTemplates(s *automation.Session, dir string) error {
	entries, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no PNG templates found in %s", dir)
	}

	paths := make(map[string]string, len(entries))
	for _, path := range entries {
		name := filepath.Base(path)
		name = name[:len(name)-len(filepath.Ext(name))]
		paths[name] = path
	}
	if err := s.LoadTemplates(paths); err != nil {
		return err
	}

	fmt.Printf("Loaded %d templates:", len(paths))
	for name := range paths {
		fmt.Printf(" %s", name)
	}
	fmt.Println()
	return nil
}

// reportFrame captures once and reports what the perception layer can read from
// the frame, including a colour measurement and a stability check.
func reportFrame(s *automation.Session) error {
	frame, err := s.Capture()
	if err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}
	fmt.Printf("Captured %dx%d px\n", frame.Width, frame.Height)

	detector := ggvimage.NewColorDetector()
	defer detector.Close()

	// A health bar is usually along the bottom edge; measure the green content of
	// that band as an illustration. Expressed as a fraction of the frame so it does
	// not hard-code one resolution.
	bottomBand := automation.RegionOf(frame, 0, 0.9, 1.0, 0.1)
	green, err := detector.Ratio(frame, bottomBand, ggvimage.RangeGreen)
	if err != nil {
		return err
	}
	red, err := detector.Ratio(frame, bottomBand, ggvimage.RangeRed)
	if err != nil {
		return err
	}
	fmt.Printf("Bottom band %v: green %.1f%%, red %.1f%%\n",
		bottomBand, green*100, red*100)

	// Wait for the screen to settle. This is how to detect "loading finished"
	// without needing a template for the loaded state.
	fmt.Print("Waiting for the frame to stabilise... ")
	if _, err := s.WaitStable(&automation.StableOptions{
		Timeout:      3 * time.Second,
		PollInterval: 100 * time.Millisecond,
	}); err != nil {
		fmt.Println("still changing (an animating window is normal)")
	} else {
		fmt.Println("stable")
	}
	return nil
}

// runMachine shows the decision layer.
//
// The states here are illustrative, but the fallbacks are the part worth copying:
// OnUnknown and OnStuck are what stop a script from spinning silently forever when
// the game shows a screen nobody planned for.
func runMachine(s *automation.Session) error {
	machine := automation.NewMachine(s, &automation.MachineOptions{
		Timeout:          30 * time.Second,
		IterationDelay:   300 * time.Millisecond,
		MaxUnknownStreak: 8,
		MaxStateStreak:   10,

		// The escape hatch for the unanticipated popup: press Escape and hope to
		// land back on familiar ground.
		OnUnknown: func(c *automation.Context) error {
			fmt.Printf("  [iteration %d] screen not recognised; pressing Escape\n", c.Iteration)
			return keyboard.Press(keyboard.KeyEscape)
		},

		OnStuck: func(c *automation.Context) error {
			fmt.Printf("  state %q is stuck after %d iterations\n", c.State, c.StateStreak)
			return keyboard.Press(keyboard.KeyEscape)
		},

		OnTransition: func(from, to string, iteration int) {
			fmt.Printf("  [iteration %d] %s -> %s\n", iteration, orNone(from), to)
		},
	})

	// A state whose detector uses a ROI. Restricting detection to the region that
	// can contain the marker is the difference between a 25 fps loop and a 6 fps
	// one, because the detector runs on every single iteration.
	frame, err := s.Capture()
	if err != nil {
		return err
	}
	topStrip := automation.RegionOf(frame, 0, 0, 1.0, 0.15)

	err = machine.AddStates(
		// Ordinary progress state. Priority 0.
		&automation.State{
			Name: "observing",
			Detect: func(c *automation.Context) (bool, error) {
				// Stands in for a real template check; always true so the example
				// has something to do.
				return true, nil
			},
			Action: func(c *automation.Context) (string, error) {
				if c.Iteration >= 3 {
					return "", automation.Done
				}
				fmt.Printf("  observing frame %dx%d in %v\n",
					c.Frame.Width, c.Frame.Height, topStrip)
				return "", nil
			},
		},

		// An interrupting state would look like this. High priority so it wins when
		// its detector and a gameplay detector both match — a modal dialog drawn
		// over the game screen matches both, and it must be handled first or the
		// script keeps trying to play through it.
		&automation.State{
			Name:     "error-dialog",
			Priority: 100,
			Detect: func(c *automation.Context) (bool, error) {
				// Would be automation.TemplateVisible("error-dialog", ...) with a
				// real template registered.
				return false, nil
			},
			Action: func(c *automation.Context) (string, error) {
				return "", keyboard.Press(keyboard.KeyEscape)
			},
		},
	)
	if err != nil {
		return err
	}

	fmt.Println("Running the state machine:")
	if err := machine.Run(); err != nil {
		return err
	}
	fmt.Printf("Done. State visits: %v\n", machine.Visits())
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(start)"
	}
	return s
}

// clickExample is kept for reference rather than called: it shows the coordinate
// path that used to be the project's sharpest edge.
//
// A match's Location is in captured-image pixels, while clicks take the platform's
// coordinate space — on a Retina display those differ by 2x. Session.ClickMatch
// does the conversion via the frame's own window snapshot, which is why it takes
// the frame rather than just the match.
func clickExample(s *automation.Session, name string) error {
	match, frame, err := s.Find(name, &ggvimage.MatchOptions{
		Threshold: 0.9,
		// Search only where the button can be, not the whole frame.
		ROI: stdimage.Rect(0, 0, 400, 300),
	})
	if err != nil {
		return err
	}

	// Clicks the centre of the match, not its top-left corner: a template's corner
	// often lands on a transparent edge and misses the actual control.
	return s.ClickMatch(frame, match, mouse.LeftButton)
}
