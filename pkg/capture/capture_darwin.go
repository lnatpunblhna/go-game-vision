//go:build darwin

package capture

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"time"

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
	tempFile := fmt.Sprintf("/tmp/window_capture_%d_%d.png", pid, time.Now().UnixNano())
	defer os.Remove(tempFile)

	// 尝试使用真实窗口ID截图
	cmd := exec.Command("screencapture", "-l", fmt.Sprintf("%d", pid), "-x", tempFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		utils.Warn("使用窗口ID截图失败: %v, %s", err, string(output))
	}

	// 检查文件是否存在且有内容
	if stat, err := os.Stat(tempFile); err != nil || stat.Size() == 0 {
		utils.Warn("窗口截图文件不存在或为空，尝试使用屏幕截图...")
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
