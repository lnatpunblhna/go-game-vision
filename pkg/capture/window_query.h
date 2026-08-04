// CoreGraphics window lookup helpers for macOS.
//
// These live in a real .h/.m pair rather than inside a Go cgo preamble because
// more than one Go file needs them: a function defined in one file's preamble is
// not visible to another's, so keeping them in a comment block forced either
// duplicate definitions or fragile extern declarations.

#ifndef WINDOW_QUERY_H
#define WINDOW_QUERY_H

#include <stdbool.h>

// getWindowIDByPID returns the CoreGraphics window ID of the first on-screen
// window owned by the given process, or -1 if it has none.
long getWindowIDByPID(int pid);

// getWindowInfo reads a window's position and size in logical points.
bool getWindowInfo(long windowID, int* width, int* height, int* x, int* y);

// isWindowValid reports whether the window still exists.
bool isWindowValid(long windowID);

// getWindowScaleFactor returns the physical-pixels-per-logical-point ratio of
// the display hosting the window (2.0 on Retina, 1.0 otherwise).
//
// This is what keeps click coordinates correct: window geometry and CGEventPost
// both use logical points while captures are in physical pixels, so without this
// ratio a match found in a Retina capture maps to a click at roughly twice the
// intended offset from the window origin. Falls back to 1.0 on any failure —
// better to skip scaling than to apply a wrong factor.
double getWindowScaleFactor(long windowID);

// captureWindowByID captures a window to a PNG file using the `screencapture`
// command. Legacy fallback for when ScreenCaptureKit is unavailable; forks a
// process and does disk IO, so it is far too slow for a capture loop.
bool captureWindowByID(long windowID, const char* outputPath);

// Window enumeration, used to find a window by title rather than by PID.
//
// Split into a snapshot call plus indexed accessors so that no allocated memory
// crosses the cgo boundary: the alternative — returning an array of structs
// containing C strings — would leave the Go side responsible for freeing memory
// the C side allocated, which is easy to get wrong and hard to notice.
//
// The snapshot is process-global and guarded by a lock, so concurrent enumeration
// is safe but the index passed to windowSnapshotAt must come from the count
// returned by the matching enumerateWindows call.

// enumerateWindows takes a fresh snapshot and returns how many windows it holds,
// or -1 on failure.
int enumerateWindows(bool includeOffscreen);

// windowSnapshotAt reads one entry from the current snapshot.
//
// title and owner are filled as NUL-terminated UTF-8, truncated to their
// capacities. Returns false if index is out of range.
bool windowSnapshotAt(int index,
                      long* windowID, int* pid, int* layer,
                      int* x, int* y, int* width, int* height,
                      char* title, int titleCap,
                      char* owner, int ownerCap);

#endif
