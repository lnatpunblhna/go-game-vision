//go:build windows

package capture

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"syscall"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procBitBlt                   = gdi32.NewProc("BitBlt")
	procGetDIBits                = gdi32.NewProc("GetDIBits")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
	procPrintWindow              = user32.NewProc("PrintWindow")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
)

const (
	SRCCOPY              = 0x00CC0020
	DIB_RGB_COLORS       = 0
	PW_CLIENTONLY        = 0x00000001
	PW_RENDERFULLCONTENT = 0x00000002
	SM_CXSCREEN          = 0
	SM_CYSCREEN          = 1
)

type RECT struct {
	Left, Top, Right, Bottom int32
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

// WindowsCapture Windows platform screenshot implementation
type WindowsCapture struct{}

// newPlatformCapture creates platform-specific capture instance
func newPlatformCapture() ScreenCapture {
	return &WindowsCapture{}
}

// CaptureWindowByPID captures window by process ID
func (w *WindowsCapture) CaptureWindowByPID(pid uint32, options *CaptureOptions) (image.Image, error) {
	windowsByPID, err := w.GetWindowsByPID(pid)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get windowsByPID")
	}

	if len(windowsByPID) == 0 {
		return nil, utils.ErrWindowNotFound
	}

	// Use main window or first visible window
	var targetWindow *WindowInfo
	for _, win := range windowsByPID {
		if w.isWindowVisible(win.Handle) {
			targetWindow = &win
			break
		}
	}

	if targetWindow == nil {
		targetWindow = &windowsByPID[0]
	}

	return w.CaptureWindowByHandle(targetWindow.Handle, options)
}

// CaptureWindowByHandle captures window by window handle
func (w *WindowsCapture) CaptureWindowByHandle(handle uintptr, options *CaptureOptions) (image.Image, error) {
	var rect RECT
	ret, _, _ := procGetWindowRect.Call(handle, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return nil, fmt.Errorf("failed to get window rectangle")
	}

	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid window size: %dx%d", width, height)
	}

	// Use PrintWindow API to capture obscured windows
	if options.IncludeHidden {
		return w.captureWindowWithPrintWindow(handle, width, height)
	}

	return w.captureWindowWithBitBlt(handle, width, height)
}

// captureWindowWithPrintWindow Using PrintWindow API to take screenshots (supports obscured windows)
func (w *WindowsCapture) captureWindowWithPrintWindow(handle uintptr, width, height int) (image.Image, error) {
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	defer procDeleteObject.Call(bitmap)

	procSelectObject.Call(memDC, bitmap)

	// Use PrintWindow to capture window content
	ret, _, _ := procPrintWindow.Call(handle, memDC, PW_RENDERFULLCONTENT)
	if ret == 0 {
		utils.Warn("PrintWindow failed, trying BitBlt")
		return w.captureWindowWithBitBlt(handle, width, height)
	}

	return w.bitmapToImage(bitmap, width, height)
}

// captureWindowWithBitBlt Taking screenshots using the BitBlt API
func (w *WindowsCapture) captureWindowWithBitBlt(handle uintptr, width, height int) (image.Image, error) {
	windowDC, _, _ := procGetDC.Call(handle)
	defer procReleaseDC.Call(handle, windowDC)

	memDC, _, _ := procCreateCompatibleDC.Call(windowDC)
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(windowDC, uintptr(width), uintptr(height))
	defer procDeleteObject.Call(bitmap)

	procSelectObject.Call(memDC, bitmap)

	ret, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(width), uintptr(height), windowDC, 0, 0, SRCCOPY)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	return w.bitmapToImage(bitmap, width, height)
}

// bitmapToImage Convert Windows bitmap to Go image
func (w *WindowsCapture) bitmapToImage(bitmap uintptr, width, height int) (image.Image, error) {
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	var bi BITMAPINFO
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = int32(width)
	bi.BmiHeader.BiHeight = -int32(height) // 负值表示从上到下
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = 0

	bufferSize := width * height * 4
	buffer := make([]byte, bufferSize)

	ret, _, _ := procGetDIBits.Call(
		hdc,
		bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&bi)),
		DIB_RGB_COLORS,
	)

	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	// Create RGBA image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			b := buffer[offset]
			g := buffer[offset+1]
			r := buffer[offset+2]
			a := buffer[offset+3]
			img.Set(x, y, color.RGBA{r, g, b, a})
		}
	}

	return img, nil
}

// GetWindowsByPID gets all windows by process ID
func (w *WindowsCapture) GetWindowsByPID(pid uint32) ([]WindowInfo, error) {
	var windowInfos []WindowInfo

	callback := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))

		if windowPID == pid {
			title := w.getWindowTitle(hwnd)
			rect := w.getWindowRect(hwnd)

			windowInfos = append(windowInfos, WindowInfo{
				Handle: hwnd,
				Title:  title,
				PID:    pid,
				Rect:   rect,
			})
		}
		return 1 // 继续枚举
	})

	procEnumWindows.Call(callback, 0)
	return windowInfos, nil
}

// SaveImage saves image to file
func (w *WindowsCapture) SaveImage(img image.Image, filename string, format ImageFormat, quality int) error {
	file, err := os.Create(filename)
	if err != nil {
		return utils.WrapError(err, "failed to create file")
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

// Helper methods
func (w *WindowsCapture) getWindowTitle(hwnd uintptr) string {
	buf := make([]uint16, 256)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	return windows.UTF16ToString(buf)
}

func (w *WindowsCapture) getWindowRect(hwnd uintptr) image.Rectangle {
	var rect RECT
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	return image.Rect(int(rect.Left), int(rect.Top), int(rect.Right), int(rect.Bottom))
}

func (w *WindowsCapture) isWindowVisible(hwnd uintptr) bool {
	ret, _, _ := procIsWindowVisible.Call(hwnd)
	return ret != 0
}
