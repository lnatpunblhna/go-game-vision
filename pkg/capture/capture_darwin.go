//go:build darwin

package capture

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"

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
	windows, err := d.GetWindowsByPID(pid)
	if err != nil {
		return nil, utils.WrapError(err, "Failed to get window")
	}

	if len(windows) == 0 {
		return nil, utils.ErrWindowNotFound
	}

	// Use the main window
	mainWindow := &windows[0]
	return d.CaptureWindowByHandle(mainWindow.Handle, options)
}

// CaptureWindowByHandle Capture window based on window handle
func (d *DarwinCapture) CaptureWindowByHandle(handle uintptr, options *CaptureOptions) (image.Image, error) {
	// On macOS, we use the screencapture command to capture a specific window
	// Due to macOS security restrictions, it is more complicated to capture screenshots directly using the window ID
	// Here is a basic implementation. In actual projects, you may need to use Core Graphics APIs such as CGWindowListCreateImage

	tempFile := fmt.Sprintf("/tmp/window_capture_%d.png", handle)
	defer os.Remove(tempFile)

	// 使用screencapture命令截取窗口
	cmd := exec.Command("screencapture", "-l", fmt.Sprintf("%d", handle), tempFile)
	err := cmd.Run()
	if err != nil {
		return nil, utils.WrapError(err, "执行screencapture命令失败")
	}

	// 读取截图文件
	file, err := os.Open(tempFile)
	if err != nil {
		return nil, utils.WrapError(err, "打开截图文件失败")
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, utils.WrapError(err, "解码图片失败")
	}

	return img, nil
}

// CaptureScreen Capture the entire screen
func (d *DarwinCapture) CaptureScreen(options *CaptureOptions) (image.Image, error) {
	tempFile := "/tmp/screen_capture.png"
	defer os.Remove(tempFile)

	// Use the screencapture command to capture the entire screen
	cmd := exec.Command("screencapture", tempFile)
	err := cmd.Run()
	if err != nil {
		return nil, utils.WrapError(err, "Failed to execute screencapture command")
	}

	// Reading screenshot files
	file, err := os.Open(tempFile)
	if err != nil {
		return nil, utils.WrapError(err, "Failed to open the screenshot file")
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, utils.WrapError(err, "Decoding picture failed")
	}

	return img, nil
}

// GetWindowsByPID Get all windows by process ID
func (d *DarwinCapture) GetWindowsByPID(pid uint32) ([]WindowInfo, error) {
	// Using osascript to get window information
	script := fmt.Sprintf(`
		tell application "System Events"
			set processName to name of (first process whose unix id is %d)
			tell process processName
				set windowList to every window
				repeat with w in windowList
					set windowTitle to title of w
					set windowPosition to position of w
					set windowSize to size of w
					return windowTitle & "|" & (item 1 of windowPosition) & "|" & (item 2 of windowPosition) & "|" & (item 1 of windowSize) & "|" & (item 2 of windowSize)
				end repeat
			end tell
		end tell
	`, pid)

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		// If the AppleScript fails, return basic window information
		utils.Warn("Failed to obtain window information, using default value: %v", err)
		return []WindowInfo{{
			Handle: uintptr(pid), // Using PID as handle
			Title:  fmt.Sprintf("Process_%d", pid),
			PID:    pid,
			Rect:   image.Rect(0, 0, 800, 600),
		}}, nil
	}

	// parse output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var windows []WindowInfo

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			title := parts[0]
			x, _ := strconv.Atoi(parts[1])
			y, _ := strconv.Atoi(parts[2])
			width, _ := strconv.Atoi(parts[3])
			height, _ := strconv.Atoi(parts[4])

			windows = append(windows, WindowInfo{
				Handle: uintptr(len(windows) + 1), // Simple handle generation
				Title:  title,
				PID:    pid,
				Rect:   image.Rect(x, y, x+width, y+height),
			})
		}
	}

	if len(windows) == 0 {
		// If no window is found, a default window is returned.
		windows = append(windows, WindowInfo{
			Handle: uintptr(pid),
			Title:  fmt.Sprintf("Process_%d", pid),
			PID:    pid,
			Rect:   image.Rect(0, 0, 800, 600),
		})
	}

	return windows, nil
}

// GetMainWindowByPID Get the main window based on the process ID
func (d *DarwinCapture) GetMainWindowByPID(pid uint32) (*WindowInfo, error) {
	windows, err := d.GetWindowsByPID(pid)
	if err != nil {
		return nil, err
	}

	if len(windows) == 0 {
		return nil, utils.ErrWindowNotFound
	}

	// Return the first window as the main window
	return &windows[0], nil
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
