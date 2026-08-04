//go:build darwin

// CoreGraphics window lookup helpers. See window_query.h.
//
// The //go:build constraint is required: build constraints in .m files are
// honoured by the go tool, and without it a GOOS=windows build of this package
// fails with "Objective-C source files not allowed when not using cgo or SWIG",
// because that build excludes every cgo .go file and the package stops being a
// cgo package.

#import <CoreGraphics/CoreGraphics.h>
#import <CoreFoundation/CoreFoundation.h>
#include <math.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "window_query.h"

long getWindowIDByPID(int pid) {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
        kCGNullWindowID
    );
    if (!windowList) return -1;

    CFIndex count = CFArrayGetCount(windowList);
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
        CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);
        if (!pidRef) continue;

        int windowPID;
        CFNumberGetValue(pidRef, kCFNumberIntType, &windowPID);
        if (windowPID != pid) continue;

        CFNumberRef windowNumber = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
        if (windowNumber) {
            long winID;
            CFNumberGetValue(windowNumber, kCFNumberLongType, &winID);
            CFRelease(windowList);
            return winID;
        }
    }

    CFRelease(windowList);
    return -1;
}

bool getWindowInfo(long windowID, int* width, int* height, int* x, int* y) {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionIncludingWindow, (CGWindowID)windowID);
    if (!windowList) return false;

    bool ok = false;
    if (CFArrayGetCount(windowList) > 0) {
        CFDictionaryRef windowDict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, 0);
        CFDictionaryRef boundsDict = (CFDictionaryRef)CFDictionaryGetValue(windowDict, kCGWindowBounds);

        if (boundsDict) {
            CFNumberRef xRef = (CFNumberRef)CFDictionaryGetValue(boundsDict, CFSTR("X"));
            CFNumberRef yRef = (CFNumberRef)CFDictionaryGetValue(boundsDict, CFSTR("Y"));
            CFNumberRef widthRef = (CFNumberRef)CFDictionaryGetValue(boundsDict, CFSTR("Width"));
            CFNumberRef heightRef = (CFNumberRef)CFDictionaryGetValue(boundsDict, CFSTR("Height"));

            if (xRef && yRef && widthRef && heightRef) {
                CFNumberGetValue(xRef, kCFNumberIntType, x);
                CFNumberGetValue(yRef, kCFNumberIntType, y);
                CFNumberGetValue(widthRef, kCFNumberIntType, width);
                CFNumberGetValue(heightRef, kCFNumberIntType, height);
                ok = true;
            }
        }
    }

    CFRelease(windowList);
    return ok;
}

bool isWindowValid(long windowID) {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionIncludingWindow, (CGWindowID)windowID);
    bool isValid = (windowList != NULL && CFArrayGetCount(windowList) > 0);
    if (windowList) CFRelease(windowList);
    return isValid;
}

double getWindowScaleFactor(long windowID) {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionIncludingWindow, (CGWindowID)windowID);
    CGRect bounds = CGRectNull;

    if (windowList && CFArrayGetCount(windowList) > 0) {
        CFDictionaryRef windowDict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, 0);
        CFDictionaryRef boundsDict = (CFDictionaryRef)CFDictionaryGetValue(windowDict, kCGWindowBounds);
        if (boundsDict) {
            CGRectMakeWithDictionaryRepresentation(boundsDict, &bounds);
        }
    }
    if (windowList) CFRelease(windowList);

    // Find the display hosting this window; on a mixed-DPI multi-monitor setup
    // the answer differs per display, so using the main display unconditionally
    // would be wrong for any window dragged to the secondary screen.
    CGDirectDisplayID display = CGMainDisplayID();
    if (!CGRectIsNull(bounds) && !CGRectIsEmpty(bounds)) {
        CGDirectDisplayID ids[8];
        uint32_t matched = 0;
        if (CGGetDisplaysWithRect(bounds, 8, ids, &matched) == kCGErrorSuccess && matched > 0) {
            display = ids[0];
        }
    }

    // Compare the mode's point width against its pixel width. Staying inside the
    // CG coordinate space avoids NSScreen's bottom-left origin and the class of
    // flip bugs that come with converting between the two.
    CGDisplayModeRef mode = CGDisplayCopyDisplayMode(display);
    if (!mode) return 1.0;
    size_t pointW = CGDisplayModeGetWidth(mode);
    size_t pixelW = CGDisplayModeGetPixelWidth(mode);
    CGDisplayModeRelease(mode);

    if (pointW == 0 || pixelW == 0) return 1.0;
    double scale = (double)pixelW / (double)pointW;
    return scale < 1.0 ? 1.0 : scale;
}

bool captureWindowByID(long windowID, const char* outputPath) {
    // -o suppresses the window's drop shadow. Without it the PNG is inflated by
    // roughly 34pt on each side, so the image origin no longer coincides with the
    // window origin and every derived click is offset by that margin. Measured:
    // a 1073pt-wide window came out 2282px instead of 2146px (2.127x, not 2.0x).
    char command[512];
    snprintf(command, sizeof(command), "screencapture -o -l%ld -x '%s'", windowID, outputPath);

    int result = system(command);
    return result == 0;
}

// --- window enumeration -----------------------------------------------------

// Snapshot state. Global rather than caller-allocated so that no malloc'd memory
// crosses the cgo boundary; see window_query.h.
typedef struct {
    long windowID;
    int pid;
    int layer;
    int x, y, width, height;
    char title[256];
    char owner[256];
} WindowEntry;

static WindowEntry* gSnapshot = NULL;
static int gSnapshotCount = 0;
static pthread_mutex_t gSnapshotLock = PTHREAD_MUTEX_INITIALIZER;

// copyCFStringToBuffer writes a CFString as UTF-8 into a fixed buffer.
//
// Always NUL-terminates, even when truncating: a window title is untrusted input
// as far as this code is concerned, and an unterminated buffer would be read past
// its end by the Go side.
static void copyCFStringToBuffer(CFStringRef str, char* buf, size_t cap) {
    if (cap == 0) return;
    buf[0] = '\0';
    if (str == NULL) return;

    if (!CFStringGetCString(str, buf, (CFIndex)cap, kCFStringEncodingUTF8)) {
        // Truncation or an unconvertible character; leave whatever was written
        // but guarantee termination.
        buf[cap - 1] = '\0';
    }
}

int enumerateWindows(bool includeOffscreen) {
    CGWindowListOption options = kCGWindowListExcludeDesktopElements;
    if (!includeOffscreen) {
        options |= kCGWindowListOptionOnScreenOnly;
    }

    CFArrayRef list = CGWindowListCopyWindowInfo(options, kCGNullWindowID);
    if (!list) return -1;

    CFIndex count = CFArrayGetCount(list);

    pthread_mutex_lock(&gSnapshotLock);

    free(gSnapshot);
    gSnapshot = NULL;
    gSnapshotCount = 0;

    if (count > 0) {
        gSnapshot = (WindowEntry*)calloc((size_t)count, sizeof(WindowEntry));
        if (gSnapshot == NULL) {
            pthread_mutex_unlock(&gSnapshotLock);
            CFRelease(list);
            return -1;
        }
    }

    int written = 0;
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        WindowEntry* e = &gSnapshot[written];

        CFNumberRef numberRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
        if (!numberRef) continue;
        CFNumberGetValue(numberRef, kCFNumberLongType, &e->windowID);

        CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);
        if (pidRef) CFNumberGetValue(pidRef, kCFNumberIntType, &e->pid);

        CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowLayer);
        if (layerRef) CFNumberGetValue(layerRef, kCFNumberIntType, &e->layer);

        CFDictionaryRef boundsDict = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
        if (boundsDict) {
            CGRect r;
            if (CGRectMakeWithDictionaryRepresentation(boundsDict, &r)) {
                e->x = (int)llround(r.origin.x);
                e->y = (int)llround(r.origin.y);
                e->width = (int)llround(r.size.width);
                e->height = (int)llround(r.size.height);
            }
        }

        // kCGWindowName is frequently absent: reading it requires screen-recording
        // permission, so without that grant every title comes back empty and only
        // the owner name is usable for matching.
        copyCFStringToBuffer((CFStringRef)CFDictionaryGetValue(dict, kCGWindowName),
                             e->title, sizeof(e->title));
        copyCFStringToBuffer((CFStringRef)CFDictionaryGetValue(dict, kCGWindowOwnerName),
                             e->owner, sizeof(e->owner));

        written++;
    }

    gSnapshotCount = written;
    pthread_mutex_unlock(&gSnapshotLock);

    CFRelease(list);
    return written;
}

bool windowSnapshotAt(int index,
                      long* windowID, int* pid, int* layer,
                      int* x, int* y, int* width, int* height,
                      char* title, int titleCap,
                      char* owner, int ownerCap) {
    pthread_mutex_lock(&gSnapshotLock);

    if (index < 0 || index >= gSnapshotCount || gSnapshot == NULL) {
        pthread_mutex_unlock(&gSnapshotLock);
        return false;
    }

    WindowEntry* e = &gSnapshot[index];
    if (windowID) *windowID = e->windowID;
    if (pid) *pid = e->pid;
    if (layer) *layer = e->layer;
    if (x) *x = e->x;
    if (y) *y = e->y;
    if (width) *width = e->width;
    if (height) *height = e->height;

    if (title && titleCap > 0) {
        strncpy(title, e->title, (size_t)titleCap - 1);
        title[titleCap - 1] = '\0';
    }
    if (owner && ownerCap > 0) {
        strncpy(owner, e->owner, (size_t)ownerCap - 1);
        owner[ownerCap - 1] = '\0';
    }

    pthread_mutex_unlock(&gSnapshotLock);
    return true;
}
