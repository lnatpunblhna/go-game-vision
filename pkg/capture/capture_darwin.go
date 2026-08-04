//go:build darwin

package capture

/*
	#cgo CFLAGS: -x objective-c -fobjc-arc
	#cgo LDFLAGS: -framework CoreGraphics -framework Foundation -framework AppKit
	#include <stdlib.h>
	#include "window_query.h"
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
	var windowID C.long

	// A title narrows the choice when the process owns several windows; without it
	// the pick falls to whatever CGWindowList happens to enumerate first, which is
	// not stable across launches.
	if options != nil && options.WindowTitle != "" {
		win, err := FindWindow(&WindowQuery{PID: pid, TitleContains: options.WindowTitle})
		if err != nil {
			return nil, utils.WrapError(err, fmt.Sprintf(
				"no window of PID %d has a title containing %q", pid, options.WindowTitle))
		}
		windowID = C.long(win.Handle)
	} else {
		windowID = C.getWindowIDByPID(C.int(pid))
	}

	if windowID == -1 {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("无法找到PID %d对应的窗口", pid))
	}

	utils.Debug("找到PID %d的窗口ID: %d", pid, windowID)

	// 检查窗口是否有效
	if !bool(C.isWindowValid(C.long(windowID))) {
		return nil, utils.WrapError(utils.ErrWindowNotFound, fmt.Sprintf("窗口ID %d无效或已关闭", windowID))
	}

	scale := float64(C.getWindowScaleFactor(C.long(windowID)))

	// 优先走 ScreenCaptureKit：直接拿到内存位图，无子进程、无 PNG 编解码、无磁盘 IO。
	// 下面的 screencapture 路径每帧要几百毫秒，只能作为兜底（SCK 不可用、
	// 或屏幕录制权限尚未授予时）。
	if img, err := captureWindowSCK(uintptr(windowID), scale); err == nil {
		utils.Debug("SCK 截取窗口 PID %d 成功: %dx%d px (scale=%.2f)",
			pid, img.Bounds().Dx(), img.Bounds().Dy(), scale)
		return img, nil
	} else {
		utils.Warn("ScreenCaptureKit 截图失败，回退 screencapture: %v", err)
	}

	return d.captureWindowViaSubprocess(pid, windowID)
}

// captureWindowViaSubprocess is the legacy `screencapture -l<id>` fallback.
//
// Retained only for the cases ScreenCaptureKit cannot serve (macOS < 14, or a
// window SCK refuses to enumerate). It forks a process, encodes a PNG, writes it
// to /tmp and decodes it back — far too slow for an automation loop, but better
// than failing outright.
func (d *DarwinCapture) captureWindowViaSubprocess(pid uint32, windowID C.long) (image.Image, error) {
	// 获取窗口信息
	var width, height, x, y C.int
	if !bool(C.getWindowInfo(windowID, &width, &height, &x, &y)) {
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

	// 求出该窗口所在屏幕的缩放系数。Rect 是逻辑点,截图是物理像素,
	// 少了这一步,Retina 上的点击坐标会成倍偏移。
	scale := float64(C.getWindowScaleFactor(C.long(windowID)))

	// 构建窗口信息结构
	windowInfo := &WindowInfo{
		Handle: uintptr(windowID),
		PID:    pid,
		Rect: image.Rectangle{
			Min: image.Point{X: int(x), Y: int(y)},
			Max: image.Point{X: int(x + width), Y: int(y + height)},
		},
		IsHidden:    false, // 通过CGWindowListCopyWindowInfo查到的窗口通常是可见的
		ScaleFactor: scale,
	}

	utils.Debug("获取到窗口信息: ID=%d, 位置(%d, %d), 尺寸(%d x %d), 缩放=%.2f",
		windowID, x, y, width, height, scale)

	return windowInfo, nil
}
