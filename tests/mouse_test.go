package tests

import (
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func init() {
	// Set up logger for tests
	utils.SetLogLevel(utils.DEBUG)
}

func TestMouseClicker_GetScreenSize(t *testing.T) {
	clicker := mouse.NewMouseClicker()

	width, height, err := clicker.GetScreenSize()
	if err != nil {
		t.Fatalf("GetScreenSize failed: %v", err)
	}

	if width <= 0 || height <= 0 {
		t.Errorf("Invalid screen size: %dx%d", width, height)
	}

	t.Logf("Screen size: %dx%d", width, height)
}

func TestMouseClicker_IsValidCoordinate(t *testing.T) {
	clicker := mouse.NewMouseClicker()

	// Test valid coordinates
	if !clicker.IsValidCoordinate(100, 100) {
		t.Error("Expected (100, 100) to be valid coordinates")
	}

	if !clicker.IsValidCoordinate(0, 0) {
		t.Error("Expected (0, 0) to be valid coordinates")
	}

	// Test invalid coordinates
	if clicker.IsValidCoordinate(-1, 100) {
		t.Error("Expected (-1, 100) to be invalid coordinates")
	}

	if clicker.IsValidCoordinate(100, -1) {
		t.Error("Expected (100, -1) to be invalid coordinates")
	}

	// Test coordinates beyond screen bounds
	width, height, err := clicker.GetScreenSize()
	if err != nil {
		t.Fatalf("Failed to get screen size: %v", err)
	}

	if clicker.IsValidCoordinate(width, height) {
		t.Errorf("Expected (%d, %d) to be invalid coordinates", width, height)
	}
}

func TestValidateCoordinates(t *testing.T) {
	// Test valid coordinates
	err := mouse.ValidateCoordinates(100, 100)
	if err != nil {
		t.Errorf("Expected (100, 100) to be valid, got error: %v", err)
	}

	// Test invalid coordinates
	err = mouse.ValidateCoordinates(-1, 100)
	if err == nil {
		t.Error("Expected (-1, 100) to be invalid")
	}

	err = mouse.ValidateCoordinates(100, -1)
	if err == nil {
		t.Error("Expected (100, -1) to be invalid")
	}
}

func TestMouseButton_String(t *testing.T) {
	tests := []struct {
		button   mouse.MouseButton
		expected string
	}{
		{mouse.LeftButton, "left"},
		{mouse.RightButton, "right"},
		{mouse.MiddleButton, "middle"},
		{mouse.MouseButton(999), "unknown"},
	}

	for _, test := range tests {
		result := test.button.String()
		if result != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, result)
		}
	}
}

func TestDefaultClickOptions(t *testing.T) {
	options := mouse.DefaultClickOptions()

	if options.Button != mouse.LeftButton {
		t.Errorf("Expected default button to be LeftButton, got %v", options.Button)
	}

	if options.Delay != 50 {
		t.Errorf("Expected default delay to be 50ms, got %d", options.Delay)
	}
}

// TestBackgroundClick_DryRun tests the click functionality without actually clicking
// This is a dry run test that validates the setup but doesn't perform actual clicks
func TestBackgroundClick_DryRun(t *testing.T) {
	clicker := mouse.NewMouseClicker()

	// Get screen size for testing
	width, height, err := clicker.GetScreenSize()
	if err != nil {
		t.Fatalf("Failed to get screen size: %v", err)
	}

	// Test coordinates within screen bounds
	testX := width / 2
	testY := height / 2

	// Validate that our test coordinates are valid
	if !clicker.IsValidCoordinate(testX, testY) {
		t.Errorf("Test coordinates (%d, %d) should be valid", testX, testY)
	}

	t.Logf("Would click at (%d, %d) on screen size %dx%d", testX, testY, width, height)
}

// TestBackgroundClick_Manual is a manual test that requires user verification
// Run this test manually when you want to verify actual clicking behavior
func TestBackgroundClick_Manual(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping manual test in short mode")
	}

	// This test is commented out by default to prevent accidental clicks during automated testing
	t.Skip("Manual test - uncomment to run actual click tests")

	/*
		clicker := mouse.NewMouseClicker()

		// Get screen center
		width, height, err := clicker.GetScreenSize()
		if err != nil {
			t.Fatalf("Failed to get screen size: %v", err)
		}

		centerX := width / 2
		centerY := height / 2

		t.Logf("Performing test clicks at screen center (%d, %d)", centerX, centerY)
		t.Log("You should see clicks happening without cursor movement")

		// Test different button types
		buttons := []mouse.MouseButton{
			mouse.LeftButton,
			mouse.RightButton,
			mouse.MiddleButton,
		}

		for _, button := range buttons {
			t.Logf("Testing %s button click", button.String())

			options := &mouse.ClickOptions{
				Button: button,
				Delay:  100,
			}

			err := clicker.BackgroundClick(centerX, centerY, options)
			if err != nil {
				t.Errorf("Failed to perform %s click: %v", button.String(), err)
			}

			// Wait between different button tests
			time.Sleep(1 * time.Second)
		}
	*/
}

// TestConvenienceFunction tests the convenience functions
func TestConvenienceFunction(t *testing.T) {
	// Test coordinate validation first
	err := mouse.ValidateCoordinates(100, 100)
	if err != nil {
		t.Fatalf("Coordinates should be valid: %v", err)
	}

	// These tests validate the function signatures and basic error handling
	// without actually performing clicks

	t.Log("Testing convenience function signatures")

	// Test with invalid coordinates to ensure error handling works
	err = mouse.BackgroundLeftClick(-1, -1)
	if err == nil {
		t.Error("Expected error for invalid coordinates")
	}

	err = mouse.BackgroundRightClick(-1, -1)
	if err == nil {
		t.Error("Expected error for invalid coordinates")
	}

	err = mouse.BackgroundMiddleClick(-1, -1)
	if err == nil {
		t.Error("Expected error for invalid coordinates")
	}
}
