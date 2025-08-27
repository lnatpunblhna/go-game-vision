//go:build darwin

package capture

/*
	#cgo CFLAGS: -x objective-c
	#cgo LDFLAGS: -framework CoreGraphics -framework Foundation -framework AppKit
	#include <CoreGraphics/CoreGraphics.h>
	#include <CoreFoundation/CoreFoundation.h>
	#include <AppKit/AppKit.h>
	#include <stdlib.h>
	#include <stdbool.h>

	// 根据PID获取窗口ID
	long getWindowIDByPID(int pid) {
		CFArrayRef windowList = CGWindowListCopyWindowInfo(
			kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
			kCGNullWindowID
		);
		CFIndex count = CFArrayGetCount(windowList);

		for (CFIndex i = 0; i < count; i++) {
			CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
			CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);

			if (pidRef) {
				int windowPID;
				CFNumberGetValue(pidRef, kCFNumberIntType, &windowPID);

				if (windowPID == pid) {
					CFNumberRef windowNumber = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
					if (windowNumber) {
						long winID;
						CFNumberGetValue(windowNumber, kCFNumberLongType, &winID);
						CFRelease(windowList);
						return winID;
					}
				}
			}
		}

		CFRelease(windowList);
		return -1;
	}

	// 根据窗口ID截取窗口内容 - 使用screencapture命令作为备用方案
	bool captureWindowByID(long windowID, const char* outputPath) {
		// 由于macOS 15中CGWindowListCreateImage被废弃，我们使用screencapture命令
		// 这虽然不是纯粹的Core Graphics方案，但在所有macOS版本上都能工作
		char command[512];
		snprintf(command, sizeof(command), "screencapture -l%ld -x '%s'", windowID, outputPath);

		int result = system(command);
		return result == 0;
	}

	// 获取窗口信息
	bool getWindowInfo(long windowID, int* width, int* height, int* x, int* y) {
		CGWindowID winID = (CGWindowID)windowID;

		CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionIncludingWindow, winID);
		if (!windowList || CFArrayGetCount(windowList) == 0) {
			if (windowList) CFRelease(windowList);
			return false;
		}

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
				CFRelease(windowList);
				return true;
			}
		}

		CFRelease(windowList);
		return false;
	}

	// 检查窗口是否存在并可见
	bool isWindowValid(long windowID) {
		CGWindowID winID = (CGWindowID)windowID;
		CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionIncludingWindow, winID);
		bool isValid = (windowList != NULL && CFArrayGetCount(windowList) > 0);
		if (windowList) CFRelease(windowList);
		return isValid;
	}
*/
import "C"
import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"time"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// DarwinCapture Screenshot implementation on macOS
type DarwinCapture struct{}

// newPlatformCapture Creating platform-specific screenshot instances
func newPlatformCapture() ScreenCapture {
	return &DarwinCapture{}
}

// CaptureWindowByPID Capture window based on process ID
func (d *DarwinCapture) CaptureWindowByPID(pid uint32, options *CaptureOptions) (image.Image, error) {
	// 首先根据PID获取窗口ID
	windowID := C.getWindowIDByPID(C.int(pid))
	if windowID == -1 {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("无法找到PID %d对应的窗口", pid))
	}

	utils.Debug("找到PID %d的窗口ID: %d", pid, windowID)

	// 检查窗口是否有效
	if !bool(C.isWindowValid(C.long(windowID))) {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("窗口ID %d无效或已关闭", windowID))
	}

	// 获取窗口信息
	var width, height, x, y C.int
	if !bool(C.getWindowInfo(C.long(windowID), &width, &height, &x, &y)) {
		utils.Warn("无法获取窗口信息，使用默认方法截图")
	} else {
		utils.Debug("窗口信息: 位置(%d, %d), 尺寸(%d x %d)", x, y, width, height)
	}

	// 创建临时文件路径
	tempFile := fmt.Sprintf("/tmp/window_capture_%d_%d.png", pid, time.Now().UnixNano())
	defer os.Remove(tempFile)

	// 使用Core Graphics API截取窗口
	cPath := C.CString(tempFile)
	defer C.free(unsafe.Pointer(cPath))

	success := bool(C.captureWindowByID(C.long(windowID), cPath))
	if !success {
		return nil, utils.WrapError(utils.ErrCaptureFailure, fmt.Sprintf("Core Graphics截图失败，窗口ID: %d", windowID))
	}

	// 检查文件是否创建成功
	stat, err := os.Stat(tempFile)
	if err != nil {
		return nil, utils.WrapError(err, "截图文件未创建")
	}

	if stat.Size() == 0 {
		return nil, utils.WrapError(utils.ErrCaptureFailure, "截图文件为空")
	}

	// 读取并解码图片
	file, err := os.Open(tempFile)
	if err != nil {
		return nil, utils.WrapError(err, "无法打开截图文件")
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, utils.WrapError(err, "图片解码失败")
	}

	utils.Info("成功截取窗口PID %d，文件大小: %d 字节", pid, stat.Size())
	return img, nil
}

// SaveImage Save image to file
func (d *DarwinCapture) SaveImage(img image.Image, filename string, format ImageFormat, quality int) error {
	file, err := os.Create(filename)
	if err != nil {
		return utils.WrapError(err, "Failed to create file")
	}
	defer file.Close()

	switch format {
	case PNG:
		err = png.Encode(file, img)
	case JPEG:
		err = jpeg.Encode(file, img, &jpeg.Options{Quality: quality})
	default:
		err = png.Encode(file, img)
	}

	if err != nil {
		return utils.WrapError(err, "failed to encode image")
	}

	utils.Info("Image saved: %s", filename)
	return nil
}

// GetWindowInfoByPID gets window information by process ID
func (d *DarwinCapture) GetWindowInfoByPID(pid uint32) (*WindowInfo, error) {
	// 根据PID获取窗口ID
	windowID := C.getWindowIDByPID(C.int(pid))
	if windowID == -1 {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("无法找到PID %d对应的窗口", pid))
	}

	// 检查窗口是否有效
	if !bool(C.isWindowValid(C.long(windowID))) {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("窗口ID %d无效或已关闭", windowID))
	}

	// 获取窗口信息
	var width, height, x, y C.int
	if !bool(C.getWindowInfo(C.long(windowID), &width, &height, &x, &y)) {
		return nil, utils.WrapError(utils.ErrCaptureFailure, "无法获取窗口详细信息")
	}

	// 构建窗口信息结构
	windowInfo := &WindowInfo{
		Handle: uintptr(windowID),
		PID:    pid,
		Rect: image.Rectangle{
			Min: image.Point{X: int(x), Y: int(y)},
			Max: image.Point{X: int(x + width), Y: int(y + height)},
		},
		IsHidden: false, // 通过CGWindowListCopyWindowInfo查到的窗口通常是可见的
	}

	utils.Debug("获取到窗口信息: ID=%d, 位置(%d, %d), 尺寸(%d x %d)",
		windowID, x, y, width, height)

	return windowInfo, nil
}
