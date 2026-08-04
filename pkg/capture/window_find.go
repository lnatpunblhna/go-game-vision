package capture

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lnatpunblhna/go-game-vision/pkg/platform"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// Finding windows by something other than PID.
//
// PID-first lookup is the wrong default on macOS, and the reason is the game
// ecosystem rather than the API. A large share of Mac games are Windows builds
// running under Wine, CrossOver or Whisky, where `ps` shows `wine64-preloader` or
// the launcher's name and the game's own name appears nowhere in the process
// table — so GetProcessPIDByName("SomeGame") simply fails. Steam-distributed
// native games often have a launcher parent process that matches the name while
// the actual game window belongs to a child.
//
// The window title, on the other hand, is set by the game itself and survives all
// of that. Hence: title first, PID as the fallback. On Windows the reverse
// ordering is still fine, but a title search is useful there too, so the query
// interface is shared.

// WindowQuery describes which windows to match. Zero-value fields are ignored, and
// all specified criteria must match.
type WindowQuery struct {
	// PID matches the owning process. Zero means any.
	PID uint32

	// TitleContains matches windows whose title contains this substring,
	// case-insensitively.
	TitleContains string

	// TitleExact matches windows whose title equals this string exactly.
	TitleExact string

	// OwnerContains matches the owning application's name, case-insensitively.
	//
	// Useful under Wine, where the owner is the Wine binary while the title is the
	// game's — matching on the owner finds every window of a Wine prefix.
	OwnerContains string

	// MinWidth and MinHeight filter out degenerate windows.
	//
	// Worth setting to something non-trivial: applications keep 1x1 and other
	// off-screen helper windows around, and those are both useless as targets and
	// frequently un-capturable.
	MinWidth  int
	MinHeight int

	// VisibleOnly restricts results to on-screen windows. Defaults to true; set
	// IncludeOffscreen to override.
	IncludeOffscreen bool

	// NormalLayerOnly restricts results to ordinary application windows,
	// excluding floating panels, tooltips and menu layers.
	//
	// Defaults to true. Turn it off only if the target genuinely draws itself in a
	// non-standard layer, since those extra windows are the usual cause of
	// "capture picked the wrong window".
	IncludeAllLayers bool
}

// Matches reports whether a window satisfies the query.
//
// Exported so callers can list once with ListWindows and then apply several
// different queries to the same snapshot, rather than re-enumerating per query.
func (q *WindowQuery) Matches(w *FoundWindow) bool {
	if q == nil || w == nil {
		return false
	}
	if q.PID != 0 && w.PID != q.PID {
		return false
	}
	if q.TitleExact != "" && w.Title != q.TitleExact {
		return false
	}
	if q.TitleContains != "" &&
		!strings.Contains(strings.ToLower(w.Title), strings.ToLower(q.TitleContains)) {
		return false
	}
	if q.OwnerContains != "" &&
		!strings.Contains(strings.ToLower(w.OwnerName), strings.ToLower(q.OwnerContains)) {
		return false
	}
	if q.MinWidth > 0 && w.Rect.Dx() < q.MinWidth {
		return false
	}
	if q.MinHeight > 0 && w.Rect.Dy() < q.MinHeight {
		return false
	}
	if !q.IncludeAllLayers && w.Layer != 0 {
		return false
	}
	return true
}

// matches is the internal form used by the platform enumerators, which have the
// pieces separately before assembling a FoundWindow.
func (q *WindowQuery) matches(w *WindowInfo, ownerName string, layer int) bool {
	return q.Matches(&FoundWindow{WindowInfo: *w, OwnerName: ownerName, Layer: layer})
}

// FoundWindow is a window returned by FindWindows, with the extra identifying
// detail that WindowInfo does not carry.
type FoundWindow struct {
	WindowInfo

	// OwnerName is the owning application's name — `wine64-preloader` for a Wine
	// game, where Title holds the game's actual name.
	OwnerName string

	// Layer is the window layer; 0 is an ordinary application window.
	Layer int
}

// String describes the window for logs and diagnostics.
func (f FoundWindow) String() string {
	return fmt.Sprintf("handle=%d pid=%d %dx%d layer=%d title=%q owner=%q",
		f.Handle, f.PID, f.Rect.Dx(), f.Rect.Dy(), f.Layer, f.Title, f.OwnerName)
}

// FindWindows returns the windows matching the query, largest first.
//
// Largest first because when several windows match, the biggest is almost always
// the main game window rather than a splash screen or helper.
func FindWindows(query *WindowQuery) ([]FoundWindow, error) {
	platform.EnsureDPIAware()

	if query == nil {
		query = &WindowQuery{}
	}

	found, err := findWindowsPlatform(query)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(found, func(i, j int) bool {
		return found[i].Rect.Dx()*found[i].Rect.Dy() > found[j].Rect.Dx()*found[j].Rect.Dy()
	})
	return found, nil
}

// FindWindow returns the single best match for the query.
//
// Fails rather than guessing when nothing matches, and returns the largest match
// when several do.
func FindWindow(query *WindowQuery) (*FoundWindow, error) {
	windows, err := FindWindows(query)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, utils.WrapError(utils.ErrWindowNotFound,
			fmt.Sprintf("no window matched %+v", query))
	}
	if len(windows) > 1 {
		utils.Debug("%d windows matched; using the largest: %s", len(windows), windows[0])
	}
	return &windows[0], nil
}

// FindWindowByTitle is the shorthand for the Wine and launcher cases: match on the
// title the game sets, ignoring whatever the process happens to be called.
func FindWindowByTitle(titleSubstring string) (*FoundWindow, error) {
	return FindWindow(&WindowQuery{
		TitleContains: titleSubstring,
		// Filter out helper and tooltip windows, which otherwise match a title
		// substring surprisingly often.
		MinWidth:  200,
		MinHeight: 100,
	})
}

// ListWindows returns every ordinary application window, largest first.
//
// The tool to reach for when a target cannot be found: printing this shows what
// the window server actually sees, including the real title and owner of a Wine
// game.
func ListWindows() ([]FoundWindow, error) {
	return FindWindows(&WindowQuery{MinWidth: 1, MinHeight: 1})
}

// NewFrameCapturerForWindow creates a capture session for a specific window
// handle, bypassing PID lookup entirely.
//
// This is what makes the title-first path complete: having found a Wine game's
// window by title, capture it directly rather than trying to work back to a PID
// that would only lead to the Wine wrapper.
func NewFrameCapturerForWindow(handle uintptr) (FrameCapturer, error) {
	platform.EnsureDPIAware()

	fc, err := newPlatformFrameCapturerForWindow(handle)
	if err != nil {
		return nil, utils.WrapError(err, "failed to create frame capturer for window")
	}
	return fc, nil
}
