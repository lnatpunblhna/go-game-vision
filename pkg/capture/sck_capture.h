// ScreenCaptureKit-backed window capture for macOS.
//
// Replaces the `screencapture -l<id>` subprocess: that path forked a process,
// encoded a PNG, wrote it to /tmp and decoded it back, costing hundreds of
// milliseconds per frame. Automation loops need in-memory frames, which is what
// this provides.
//
// Requires macOS 14+ for SCScreenshotManager; sckIsAvailable() reports whether
// the running system qualifies so the caller can fall back.

#ifndef SCK_CAPTURE_H
#define SCK_CAPTURE_H

#include <stdbool.h>

// SCKImage is a raw BGRA pixel buffer handed back to the caller.
//
// data is malloc'd and becomes the caller's responsibility to free(). Pixel
// order is BGRA with premultiplied alpha (kCGBitmapByteOrder32Little |
// kCGImageAlphaPremultipliedFirst), which maps onto Go's image.RGBA with a
// per-pixel B/R swap.
typedef struct {
    unsigned char* data;
    int width;
    int height;
    int bytesPerRow;
} SCKImage;

// sckIsAvailable reports whether ScreenCaptureKit's screenshot API is present.
bool sckIsAvailable(void);

// sckCaptureWindow captures the window with the given CoreGraphics window ID.
//
// scale is the display scale factor (2.0 on Retina); the capture is configured
// in physical pixels, so passing the wrong value yields a mis-sized frame that
// no longer agrees with WindowInfo.ScaleFactor.
//
// Returns 0 on success and fills *out. Error codes:
//   -1 window not found in shareable content (closed, or permission denied)
//   -2 window has zero size
//   -3 capture timed out or was refused
//   -4 out of memory
//   -5 failed to create bitmap context
//   -6 ScreenCaptureKit unavailable on this macOS version
int sckCaptureWindow(long windowID, double scale, SCKImage* out);

// sckCaptureWindowInto captures directly into a caller-owned buffer, avoiding a
// malloc and a copy per frame — which matters in a capture loop, where the
// alternative is churning ~16MB of garbage every iteration for a Retina window.
//
// dst may be NULL (with dstCap 0) purely to query the required geometry.
// outWidth/outHeight/outStride are filled whenever the geometry is known, even
// when the buffer is too small, so the caller can size and retry.
//
// Returns 0 on success, -7 if dstCap is smaller than outStride*outHeight, and
// otherwise the same negative codes as sckCaptureWindow.
int sckCaptureWindowInto(long windowID, double scale,
                         unsigned char* dst, int dstCap,
                         int* outWidth, int* outHeight, int* outStride);

// sckGetWindowBounds reads the window's current position and size in logical
// points, straight from the window server.
//
// Cheap enough to call every frame, which a capture loop must do: if the user
// drags the window and the caller keeps using a stale origin, every click
// computed from a match lands at the old location. Returns false if the window
// no longer exists.
bool sckGetWindowBounds(long windowID, int* x, int* y, int* width, int* height);

// sckInvalidateCache drops the cached window/filter lookup, forcing the next
// capture to re-enumerate shareable content. Enumerating costs tens of
// milliseconds, so it is cached across frames; call this when the target window
// is known to have closed or moved to another display.
void sckInvalidateCache(void);

#endif
