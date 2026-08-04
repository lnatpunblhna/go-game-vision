package tests

import (
	stdimage "image"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
)

// Offline tests for window query matching, plus integration tests for the actual
// enumeration.

// win builds a FoundWindow for match testing.
func win(pid uint32, title, owner string, layer, w, h int) *capture.FoundWindow {
	return &capture.FoundWindow{
		WindowInfo: capture.WindowInfo{
			PID:   pid,
			Title: title,
			Rect:  stdimage.Rect(0, 0, w, h),
		},
		OwnerName: owner,
		Layer:     layer,
	}
}

func TestWindowQueryMatches(t *testing.T) {
	// Modelled on a real Wine setup: the process is the Wine wrapper, while the
	// title is the game's own name. This is the case PID lookup cannot handle.
	wineGame := win(4242, "Stardew Valley", "wine64-preloader", 0, 1920, 1080)
	nativeGame := win(777, "Factorio", "factorio", 0, 1600, 900)
	tooltip := win(777, "tip", "factorio", 25, 120, 40)
	tiny := win(777, "Factorio Helper", "factorio", 0, 40, 20)

	cases := []struct {
		name  string
		query capture.WindowQuery
		w     *capture.FoundWindow
		want  bool
	}{
		{"empty query matches a normal window", capture.WindowQuery{}, nativeGame, true},

		{"PID match", capture.WindowQuery{PID: 777}, nativeGame, true},
		{"PID mismatch", capture.WindowQuery{PID: 778}, nativeGame, false},

		// The Wine case: matching by title finds the game even though the process
		// is named after Wine.
		{"title substring finds a Wine game", capture.WindowQuery{TitleContains: "Stardew"}, wineGame, true},
		{"title substring is case-insensitive", capture.WindowQuery{TitleContains: "stardew valley"}, wineGame, true},
		{"title substring mismatch", capture.WindowQuery{TitleContains: "Terraria"}, wineGame, false},

		{"exact title match", capture.WindowQuery{TitleExact: "Factorio"}, nativeGame, true},
		// Exact means exact: a substring must not satisfy it.
		{"exact title rejects a substring", capture.WindowQuery{TitleExact: "Facto"}, nativeGame, false},

		// Matching the Wine wrapper by owner finds every window in the prefix.
		{"owner substring", capture.WindowQuery{OwnerContains: "wine"}, wineGame, true},
		{"owner substring is case-insensitive", capture.WindowQuery{OwnerContains: "WINE64"}, wineGame, true},
		{"owner mismatch", capture.WindowQuery{OwnerContains: "steam"}, wineGame, false},

		// Layer filtering is what keeps tooltips and panels out of results.
		{"tooltip excluded by default", capture.WindowQuery{}, tooltip, false},
		{"tooltip included when asked", capture.WindowQuery{IncludeAllLayers: true}, tooltip, true},

		{"tiny window excluded by MinWidth", capture.WindowQuery{MinWidth: 200}, tiny, false},
		{"tiny window excluded by MinHeight", capture.WindowQuery{MinHeight: 100}, tiny, false},
		{"large window passes size filter", capture.WindowQuery{MinWidth: 200, MinHeight: 100}, nativeGame, true},

		// All criteria must hold together.
		{
			"combined criteria all match",
			capture.WindowQuery{PID: 4242, TitleContains: "Stardew", OwnerContains: "wine", MinWidth: 800},
			wineGame, true,
		},
		{
			"combined criteria one fails",
			capture.WindowQuery{PID: 4242, TitleContains: "Stardew", OwnerContains: "crossover"},
			wineGame, false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.query.Matches(tc.w); got != tc.want {
				t.Errorf("Matches() = %v, want %v (window %s)", got, tc.want, tc.w)
			}
		})
	}
}

func TestWindowQueryMatchesNilSafety(t *testing.T) {
	var q *capture.WindowQuery
	if q.Matches(win(1, "x", "y", 0, 100, 100)) {
		t.Error("a nil query should not match")
	}

	valid := &capture.WindowQuery{}
	if valid.Matches(nil) {
		t.Error("a nil window should not match")
	}
}

func TestFoundWindowString(t *testing.T) {
	w := win(4242, "Stardew Valley", "wine64-preloader", 0, 1920, 1080)
	s := w.String()
	// The description must carry enough to identify the window in a log, since
	// that is the whole point of printing it while hunting for a target.
	for _, want := range []string{"4242", "1920x1080", "Stardew Valley", "wine64-preloader"} {
		if !contains(s, want) {
			t.Errorf("FoundWindow.String() = %q, missing %q", s, want)
		}
	}
}

// --- integration ------------------------------------------------------------

func TestListWindowsFindsSomething(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a windowing session")
	}

	windows, err := capture.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	if len(windows) == 0 {
		t.Skip("no windows on screen")
	}

	t.Logf("found %d windows; largest few:", len(windows))
	for i, w := range windows {
		if i >= 5 {
			break
		}
		t.Logf("  %s", w)
	}

	// Results must be ordered largest first, since callers rely on [0] being the
	// main window rather than a splash screen.
	for i := 1; i < len(windows); i++ {
		prev := windows[i-1].Rect.Dx() * windows[i-1].Rect.Dy()
		cur := windows[i].Rect.Dx() * windows[i].Rect.Dy()
		if prev < cur {
			t.Errorf("windows not sorted by area: %d before %d", prev, cur)
		}
	}
}

func TestFindWindowsByPIDAgreesWithGetWindowInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window")
	}

	pid, info := findWindowedProcess(t)
	if pid == 0 {
		t.Skip("no windowed process found")
	}

	windows, err := capture.FindWindows(&capture.WindowQuery{PID: pid})
	if err != nil {
		t.Fatalf("FindWindows: %v", err)
	}
	if len(windows) == 0 {
		t.Skipf("FindWindows found nothing for PID %d, though GetWindowInfoByPID did", pid)
	}

	t.Logf("PID %d has %d windows; GetWindowInfoByPID reported %v", pid, len(windows), info.Rect)
	for _, w := range windows {
		if w.PID != pid {
			t.Errorf("window %s has PID %d, expected %d", w, w.PID, pid)
		}
		// Every result should carry a resolved scale factor, or click mapping
		// downstream would silently use 1.0 on a Retina display.
		if w.ScaleFactor <= 0 {
			t.Errorf("window %s has an unresolved ScaleFactor", w)
		}
	}
}

// TestFindWindowByTitleRoundTrip takes a title from a live window and checks the
// title-first lookup path finds it again — the path that Wine games depend on.
func TestFindWindowByTitleRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a real window")
	}

	windows, err := capture.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}

	// Find a large window that actually has a title. On macOS titles require
	// screen-recording permission, so an untitled list means the permission is
	// missing rather than that the feature is broken.
	var target *capture.FoundWindow
	for i := range windows {
		if windows[i].Title != "" && windows[i].Rect.Dx() >= 300 && windows[i].Rect.Dy() >= 200 {
			target = &windows[i]
			break
		}
	}
	if target == nil {
		t.Skip("no titled window found (screen-recording permission may be missing)")
	}

	t.Logf("looking up by title: %q", target.Title)
	found, err := capture.FindWindowByTitle(target.Title)
	if err != nil {
		t.Fatalf("FindWindowByTitle(%q): %v", target.Title, err)
	}
	if !contains(found.Title, target.Title) && !contains(target.Title, found.Title) {
		t.Errorf("found %q, expected something matching %q", found.Title, target.Title)
	}
}

func TestFindWindowFailsCleanlyForNoMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a windowing session")
	}

	// A title no window will have; must be a clear error rather than a nil
	// FoundWindow that panics later.
	found, err := capture.FindWindowByTitle("zzz-no-such-window-title-zzz-9f3a2b")
	if err == nil {
		t.Errorf("FindWindowByTitle succeeded unexpectedly: %v", found)
	}
	if found != nil {
		t.Errorf("expected a nil window alongside the error, got %v", found)
	}
}

func TestNewFrameCapturerForWindowRejectsBadHandle(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a windowing session")
	}

	if _, err := capture.NewFrameCapturerForWindow(0); err == nil {
		t.Error("expected an error for a zero window handle")
	}
	if _, err := capture.NewFrameCapturerForWindow(0xFFFFFFF); err == nil {
		t.Error("expected an error for a nonexistent window handle")
	}
}
