//go:build darwin

package process

/*
	#cgo CFLAGS: -x objective-c
	#cgo LDFLAGS: -framework CoreGraphics -framework Foundation
	#include <CoreGraphics/CoreGraphics.h>
	#include <CoreFoundation/CoreFoundation.h>
	#include <stdlib.h>

	char* getWindowID(const char* windowTitle) {
	    CFStringRef targetTitle = CFStringCreateWithCString(NULL, windowTitle, kCFStringEncodingUTF8);
	    CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
	    CFIndex count = CFArrayGetCount(windowList);

	    for (CFIndex i = 0; i < count; i++) {
	        CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
	        CFStringRef titleRef = (CFStringRef)CFDictionaryGetValue(dict, kCGWindowName);
	        if (titleRef && CFStringCompare(titleRef, targetTitle, 0) == kCFCompareEqualTo) {
	            CFNumberRef windowNumber = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
	            int64_t winID;
	            CFNumberGetValue(windowNumber, kCFNumberSInt64Type, &winID);
	            char* result = (char*)malloc(32);
	            snprintf(result, 32, "%lld", winID);
	            CFRelease(targetTitle);
	            CFRelease(windowList);
	            return result;
	        }
	    }

	    CFRelease(targetTitle);
	    CFRelease(windowList);
	    return NULL;
	}

	char* getWindowIDFuzzy(const char* keyword) {
		CFStringRef keywordRef = CFStringCreateWithCString(NULL, keyword, kCFStringEncodingUTF8);
		CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
		CFIndex count = CFArrayGetCount(windowList);

		for (CFIndex i = 0; i < count; i++) {
			CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
			CFStringRef titleRef = (CFStringRef)CFDictionaryGetValue(dict, kCGWindowName);
			if (titleRef) {
				CFRange found = CFStringFind(titleRef, keywordRef, kCFCompareCaseInsensitive);
				if (found.location != kCFNotFound) {
					CFNumberRef windowNumber = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
					int64_t winID;
					CFNumberGetValue(windowNumber, kCFNumberSInt64Type, &winID);
					char* result = (char*)malloc(32);
					snprintf(result, 32, "%lld", winID);
					CFRelease(keywordRef);
					CFRelease(windowList);
					return result;
				}
			}
		}

		CFRelease(keywordRef);
		CFRelease(windowList);
		return NULL;
	}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// DarwinProcessManager macOS platform process manager
type DarwinProcessManager struct{}

// newPlatformProcessManager creates platform-specific process manager
func newPlatformProcessManager() ProcessManager {
	return &DarwinProcessManager{}
}

// GetProcessByName gets process information by process name
func (d *DarwinProcessManager) GetProcessByName(name string, mode MatchMode) ([]ProcessInfo, error) {
	var result []ProcessInfo
	var proc ProcessInfo
	switch mode {
	case ExactMatch:
		u64, err := strconv.ParseUint(d.getWindowID(name), 10, 64)
		if err != nil {
			fmt.Println("转换失败:", err)
		}
		proc.PID = uint32(u64)
	case FuzzyMatch:
		u64, err := strconv.ParseUint(d.getWindowIDFuzzy(name), 10, 64)
		if err != nil {
			fmt.Println("转换失败:", err)
		}
		proc.PID = uint32(u64)
	}

	result = append(result, proc)

	utils.Debug("Found %d matching processes: %s", len(result), name)
	return result, nil
}

// GetProcessByPID gets process information by PID
func (d *DarwinProcessManager) GetProcessByPID(pid uint32) (*ProcessInfo, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "pid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		return nil, utils.WrapError(err, "failed to execute ps command")
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, utils.ErrProcessNotFound
	}

	// Parse ps output
	fields := strings.Fields(lines[1])
	if len(fields) < 3 {
		return nil, utils.ErrProcessNotFound
	}

	parsedPID, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		return nil, utils.WrapError(err, "failed to parse PID")
	}

	processName := fields[1]
	processPath := ""
	if len(fields) > 2 {
		processPath = strings.Join(fields[2:], " ")
	}

	return &ProcessInfo{
		PID:  uint32(parsedPID),
		Name: processName,
		Path: processPath,
	}, nil
}

// ListAllProcesses lists all processes
func (d *DarwinProcessManager) ListAllProcesses() ([]ProcessInfo, error) {
	cmd := exec.Command("ps", "-eo", "pid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		return nil, utils.WrapError(err, "failed to execute ps command")
	}

	lines := strings.Split(string(output), "\n")
	var processes []ProcessInfo

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			continue
		}

		processName := fields[1]
		processPath := ""
		if len(fields) > 2 {
			processPath = strings.Join(fields[2:], " ")
		}

		processes = append(processes, ProcessInfo{
			PID:  uint32(pid),
			Name: processName,
			Path: processPath,
		})
	}

	utils.Debug("Listed %d processes", len(processes))
	return processes, nil
}

// IsProcessRunning checks if process is running
func (d *DarwinProcessManager) IsProcessRunning(pid uint32) bool {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid))
	err := cmd.Run()
	return err == nil
}

func (d *DarwinProcessManager) getWindowID(title string) string {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	cID := C.getWindowID(cTitle)
	if cID == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cID))
	return C.GoString(cID)
}

func (d *DarwinProcessManager) getWindowIDFuzzy(keyword string) string {
	cKeyword := C.CString(keyword)
	defer C.free(unsafe.Pointer(cKeyword))

	cID := C.getWindowIDFuzzy(cKeyword)
	if cID == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cID))
	return C.GoString(cID)
}
