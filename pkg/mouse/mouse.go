package mouse

import (
	"fmt"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// MouseButton represents mouse button types
type MouseButton int

const (
	LeftButton   MouseButton = iota // 左键
	RightButton                     // 右键
	MiddleButton                    // 中键
)

// String returns the string representation of MouseButton
func (mb MouseButton) String() string {
	switch mb {
	case LeftButton:
		return "left"
	case RightButton:
		return "right"
	case MiddleButton:
		return "middle"
	default:
		return "unknown"
	}
}

// ClickOptions represents mouse click options
type ClickOptions struct {
	Button MouseButton // 鼠标按键类型
	Delay  int         // 点击延迟（毫秒）
}

// MouseClicker interface defines mouse clicking operations
type MouseClicker interface {
	// BackgroundClick performs a background mouse click at specified coordinates
	// x, y: screen coordinates
	// options: click options (button type, delay, etc.)
	BackgroundClick(x, y int, options *ClickOptions) error

	// GetScreenSize returns the screen dimensions
	GetScreenSize() (width, height int, err error)

	// IsValidCoordinate checks if the given coordinates are within screen bounds
	IsValidCoordinate(x, y int) bool
}

// DefaultClickOptions returns default click options
func DefaultClickOptions() *ClickOptions {
	return &ClickOptions{
		Button: LeftButton,
		Delay:  50, // 50ms default delay
	}
}

// NewMouseClicker creates a new platform-specific mouse clicker
func NewMouseClicker() MouseClicker {
	return newPlatformMouseClicker()
}

// BackgroundClick is a convenience function for performing background clicks
func BackgroundClick(x, y int, button MouseButton) error {
	clicker := NewMouseClicker()
	options := &ClickOptions{
		Button: button,
		Delay:  50,
	}

	utils.Debug("Performing background click at (%d, %d) with %s button", x, y, button.String())
	return clicker.BackgroundClick(x, y, options)
}

// BackgroundLeftClick performs a background left click at specified coordinates
func BackgroundLeftClick(x, y int) error {
	return BackgroundClick(x, y, LeftButton)
}

// BackgroundRightClick performs a background right click at specified coordinates
func BackgroundRightClick(x, y int) error {
	return BackgroundClick(x, y, RightButton)
}

// BackgroundMiddleClick performs a background middle click at specified coordinates
func BackgroundMiddleClick(x, y int) error {
	return BackgroundClick(x, y, MiddleButton)
}

// ValidateCoordinates validates if the given coordinates are within screen bounds
func ValidateCoordinates(x, y int) error {
	clicker := NewMouseClicker()
	if !clicker.IsValidCoordinate(x, y) {
		width, height, err := clicker.GetScreenSize()
		if err != nil {
			return utils.WrapError(err, "failed to get screen size for validation")
		}
		return fmt.Errorf("coordinates (%d, %d) are out of screen bounds (0, 0) to (%d, %d)",
			x, y, width-1, height-1)
	}
	return nil
}
