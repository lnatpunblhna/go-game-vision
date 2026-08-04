//go:build darwin

// ScreenCaptureKit window capture implementation. See sck_capture.h.
//
// Compiled with ARC (-fobjc-arc, set in capture_darwin_sck.go's cgo CFLAGS).
//
// The //go:build constraint above is load-bearing, not decorative: build
// constraints in .m files are honoured by the go tool, and without it a
// GOOS=windows build of this package fails outright with "Objective-C source
// files not allowed when not using cgo or SWIG" — the Windows build excludes
// every cgo .go file, so the package stops being a cgo package while this .m
// file is still sitting in the directory.

#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreVideo/CoreVideo.h>
#include <math.h>
#include <stdlib.h>

#include "sck_capture.h"

// How long to wait for the two async SCK calls before giving up. Both normally
// complete in tens of milliseconds; these bounds only exist so a wedged
// WindowServer cannot hang the calling goroutine forever.
static const double kShareableContentTimeoutSec = 3.0;
static const double kCaptureTimeoutSec = 5.0;

// Cached lookup state.
//
// SCShareableContent enumeration is the expensive part of an SCK capture (tens
// of milliseconds — enough to dominate a capture loop), while the resulting
// SCWindow and SCContentFilter stay valid as long as the window lives. So the
// lookup is cached per window ID and only the frame geometry is re-read each
// frame, which is cheap via CGWindowListCopyWindowInfo.
static CGWindowID gCachedWindowID = 0;
static SCWindow* gCachedWindow = nil;
static SCContentFilter* gCachedFilter = nil;
static SCStreamConfiguration* gCachedConfig = nil;
static NSLock* gCacheLock = nil;

static void sckInitLock(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        gCacheLock = [[NSLock alloc] init];
    });
}

bool sckIsAvailable(void) {
    if (@available(macOS 14.0, *)) {
        return [SCScreenshotManager respondsToSelector:
            @selector(captureImageWithFilter:configuration:completionHandler:)];
    }
    return false;
}

void sckInvalidateCache(void) {
    sckInitLock();
    [gCacheLock lock];
    gCachedWindowID = 0;
    gCachedWindow = nil;
    gCachedFilter = nil;
    gCachedConfig = nil;
    [gCacheLock unlock];
}

// sckReadWindowBounds reads the window's *current* bounds in logical points
// straight from the window server. Preferred over the cached SCWindow.frame,
// which is a snapshot taken when shareable content was enumerated and goes stale
// the moment the user moves or resizes the window.
static bool sckReadWindowBounds(CGWindowID windowID, CGRect* outRect) {
    CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionIncludingWindow, windowID);
    if (!list) return false;

    bool ok = false;
    if (CFArrayGetCount(list) > 0) {
        CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(list, 0);
        CFDictionaryRef bounds = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
        if (bounds && CGRectMakeWithDictionaryRepresentation(bounds, outRect)) {
            ok = true;
        }
    }
    CFRelease(list);
    return ok;
}

bool sckGetWindowBounds(long windowID, int* x, int* y, int* width, int* height) {
    CGRect rect;
    if (!sckReadWindowBounds((CGWindowID)windowID, &rect)) {
        return false;
    }
    if (x) *x = (int)llround(rect.origin.x);
    if (y) *y = (int)llround(rect.origin.y);
    if (width) *width = (int)llround(rect.size.width);
    if (height) *height = (int)llround(rect.size.height);
    return true;
}

// sckResolveWindow returns the SCWindow for the given ID, populating the cache on
// a miss. Must be called with gCacheLock held.
static SCWindow* sckResolveWindowLocked(CGWindowID target) {
    if (gCachedWindowID == target && gCachedWindow != nil) {
        return gCachedWindow;
    }

    __block SCWindow* found = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    [SCShareableContent getShareableContentWithCompletionHandler:
        ^(SCShareableContent* content, NSError* error) {
            if (error == nil && content != nil) {
                for (SCWindow* w in content.windows) {
                    if (w.windowID == target) {
                        found = w;
                        break;
                    }
                }
            }
            dispatch_semaphore_signal(sem);
        }];

    dispatch_semaphore_wait(sem,
        dispatch_time(DISPATCH_TIME_NOW, (int64_t)(kShareableContentTimeoutSec * NSEC_PER_SEC)));

    if (found == nil) {
        return nil;
    }

    gCachedWindowID = target;
    gCachedWindow = found;
    gCachedFilter = [[SCContentFilter alloc] initWithDesktopIndependentWindow:found];
    gCachedConfig = nil; // rebuilt below, since it depends on current geometry
    return found;
}

// sckAcquireImage performs the capture and hands back a retained CGImageRef that
// the caller must CGImageRelease. Shared by both public entry points.
static int sckAcquireImage(long windowID, double scale, CGImageRef* outImage) {
    if (outImage == NULL) return -2;
    *outImage = NULL;
    if (!sckIsAvailable()) return -6;
    if (scale <= 0.0) scale = 1.0;

    @autoreleasepool {
        sckInitLock();
        [gCacheLock lock];

        SCWindow* win = sckResolveWindowLocked((CGWindowID)windowID);
        if (win == nil) {
            [gCacheLock unlock];
            return -1;
        }

        // Re-read geometry every frame so a moved or resized window still yields
        // a correctly sized capture.
        CGRect bounds;
        if (!sckReadWindowBounds((CGWindowID)windowID, &bounds)) {
            bounds = win.frame;
        }

        size_t pxW = (size_t)llround(bounds.size.width * scale);
        size_t pxH = (size_t)llround(bounds.size.height * scale);
        if (pxW == 0 || pxH == 0) {
            [gCacheLock unlock];
            return -2;
        }

        SCContentFilter* filter = gCachedFilter;
        if (filter == nil) {
            [gCacheLock unlock];
            return -1;
        }

        // Rebuild the configuration only when the pixel dimensions change;
        // SCStreamConfiguration is otherwise reusable across frames.
        if (gCachedConfig == nil || gCachedConfig.width != pxW || gCachedConfig.height != pxH) {
            SCStreamConfiguration* cfg = [[SCStreamConfiguration alloc] init];
            cfg.width = pxW;
            cfg.height = pxH;
            cfg.showsCursor = NO;
            cfg.pixelFormat = kCVPixelFormatType_32BGRA;
            // Ask for the true backing resolution rather than letting SCK pick a
            // downscaled mode; template matching depends on pixel fidelity.
            if (@available(macOS 14.0, *)) {
                cfg.captureResolution = SCCaptureResolutionBest;
            }
            gCachedConfig = cfg;
        }
        SCStreamConfiguration* config = gCachedConfig;

        [gCacheLock unlock];

        __block CGImageRef captured = NULL;
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);

        if (@available(macOS 14.0, *)) {
            [SCScreenshotManager captureImageWithFilter:filter
                                          configuration:config
                                      completionHandler:^(CGImageRef image, NSError* error) {
                if (error == nil && image != NULL) {
                    captured = CGImageRetain(image);
                }
                dispatch_semaphore_signal(sem);
            }];
        } else {
            dispatch_semaphore_signal(sem);
        }

        dispatch_semaphore_wait(sem,
            dispatch_time(DISPATCH_TIME_NOW, (int64_t)(kCaptureTimeoutSec * NSEC_PER_SEC)));

        if (captured == NULL) {
            // A failure here often means the window vanished between the cached
            // lookup and now; drop the cache so the next attempt re-enumerates
            // instead of failing forever against a dead handle.
            sckInvalidateCache();
            return -3;
        }

        *outImage = captured;
        return 0;
    }
}

// sckDrawInto rasterises a CGImage into dst as BGRA.
static int sckDrawInto(CGImageRef image, unsigned char* dst, size_t stride, size_t w, size_t h) {
    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate(
        dst, w, h, 8, stride, cs,
        kCGImageAlphaPremultipliedFirst | kCGBitmapByteOrder32Little);
    CGColorSpaceRelease(cs);

    if (ctx == NULL) return -5;

    CGContextDrawImage(ctx, CGRectMake(0, 0, (CGFloat)w, (CGFloat)h), image);
    CGContextRelease(ctx);
    return 0;
}

int sckCaptureWindow(long windowID, double scale, SCKImage* out) {
    if (out == NULL) return -2;

    CGImageRef image = NULL;
    int rc = sckAcquireImage(windowID, scale, &image);
    if (rc != 0) return rc;

    size_t w = CGImageGetWidth(image);
    size_t h = CGImageGetHeight(image);
    size_t stride = w * 4;

    unsigned char* buf = (unsigned char*)malloc(stride * h);
    if (buf == NULL) {
        CGImageRelease(image);
        return -4;
    }

    rc = sckDrawInto(image, buf, stride, w, h);
    CGImageRelease(image);
    if (rc != 0) {
        free(buf);
        return rc;
    }

    out->data = buf;
    out->width = (int)w;
    out->height = (int)h;
    out->bytesPerRow = (int)stride;
    return 0;
}

int sckCaptureWindowInto(long windowID, double scale,
                         unsigned char* dst, int dstCap,
                         int* outWidth, int* outHeight, int* outStride) {
    CGImageRef image = NULL;
    int rc = sckAcquireImage(windowID, scale, &image);
    if (rc != 0) return rc;

    size_t w = CGImageGetWidth(image);
    size_t h = CGImageGetHeight(image);
    size_t stride = w * 4;

    // Report the geometry unconditionally: on a -7 the caller needs these
    // numbers to size its buffer before retrying.
    if (outWidth) *outWidth = (int)w;
    if (outHeight) *outHeight = (int)h;
    if (outStride) *outStride = (int)stride;

    if (dst == NULL || dstCap < 0 || (size_t)dstCap < stride * h) {
        CGImageRelease(image);
        return -7;
    }

    rc = sckDrawInto(image, dst, stride, w, h);
    CGImageRelease(image);
    return rc;
}
