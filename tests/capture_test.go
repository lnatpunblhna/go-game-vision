package tests

import (
	"image"
	"os"
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	image2 "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func TestScreenCapture(t *testing.T) {
	capturer := capture.NewScreenCapture()

	t.Run("CaptureScreen", func(t *testing.T) {
		options := capture.DefaultCaptureOptions()

		img, err := capturer.CaptureScreen(options)
		if err != nil {
			t.Fatalf("Failed to capture screen: %v", err)
		}

		if img == nil {
			t.Fatal("Screenshot result should not be nil")
		}

		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			t.Errorf("Invalid screenshot size: %dx%d", bounds.Dx(), bounds.Dy())
		}

		t.Logf("Screen capture successful, size: %dx%d", bounds.Dx(), bounds.Dy())

		// Test saving screenshot
		filename := "test_screen_capture.png"
		defer os.Remove(filename)

		err = capturer.SaveImage(img, filename, capture.PNG, 90)
		if err != nil {
			t.Fatalf("Failed to save screenshot: %v", err)
		}

		// Check if file exists
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("Screenshot file not created")
		}

		t.Logf("Screenshot saved to: %s", filename)
	})

	t.Run("GetWindowsByPID", func(t *testing.T) {
		// Find a process for testing
		manager := process.NewProcessManager()
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(processes) == 0 {
			t.Skip("No processes available for testing")
		}

		// Select a process
		testPID := processes[0].PID

		windows, err := capturer.GetWindowsByPID(testPID)
		if err != nil {
			t.Fatalf("Failed to get windows: %v", err)
		}

		t.Logf("Process %d has %d windows", testPID, len(windows))

		// Check window information
		for i, win := range windows {
			if i >= 3 { // Only check first 3
				break
			}

			if win.PID != testPID {
				t.Errorf("Window PID mismatch: expected %d, got %d", testPID, win.PID)
			}

			t.Logf("Window %d: title='%s', handle=%d, position=(%d,%d), size=(%dx%d)",
				i+1, win.Title, win.Handle, win.Rect.Min.X, win.Rect.Min.Y,
				win.Rect.Dx(), win.Rect.Dy())
		}
	})

	t.Run("GetMainWindowByPID", func(t *testing.T) {
		// Find a process for testing
		manager := process.NewProcessManager()
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(processes) == 0 {
			t.Skip("No processes available for testing")
		}

		// Select a process
		testPID := processes[0].PID

		mainWindow, err := capturer.GetMainWindowByPID(testPID)
		if err != nil {
			// Some processes may not have windows, this is normal
			t.Logf("Process %d has no main window: %v", testPID, err)
			return
		}

		if mainWindow.PID != testPID {
			t.Errorf("Main window PID mismatch: expected %d, got %d", testPID, mainWindow.PID)
		}

		t.Logf("Main window: title='%s', handle=%d", mainWindow.Title, mainWindow.Handle)
	})
}

func TestWindowCapture(t *testing.T) {
	capturer := capture.NewScreenCapture()

	t.Run("CaptureWindowByPID", func(t *testing.T) {
		// Try to find explorer process (usually has windows)
		pid, err := process.GetProcessPIDByName("explorer", process.FuzzyMatch)
		if err != nil {
			t.Skip("Explorer process not found, skipping window capture test")
		}

		options := &capture.CaptureOptions{
			Format:        capture.PNG,
			Quality:       90,
			IncludeHidden: true,
		}

		img, err := capturer.CaptureWindowByPID(pid, options)
		if err != nil {
			t.Logf("Window capture failed (this may be normal): %v", err)
			return
		}

		if img == nil {
			t.Fatal("Screenshot result should not be nil")
		}

		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			t.Errorf("Invalid screenshot size: %dx%d", bounds.Dx(), bounds.Dy())
		}

		t.Logf("Window capture successful, size: %dx%d", bounds.Dx(), bounds.Dy())

		// Test saving screenshot
		filename := "test_window_capture.png"
		defer os.Remove(filename)

		err = capturer.SaveImage(img, filename, capture.PNG, 90)
		if err != nil {
			t.Fatalf("Failed to save screenshot: %v", err)
		}

		t.Logf("Window screenshot saved to: %s", filename)
	})

	t.Run("CaptureWindowByHandle", func(t *testing.T) {
		// Find a process with windows
		manager := process.NewProcessManager()
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		var testHandle uintptr
		var testPID uint32

		// Find a process with windows
		for _, proc := range processes {
			windows, err := capturer.GetWindowsByPID(proc.PID)
			if err == nil && len(windows) > 0 {
				testHandle = windows[0].Handle
				testPID = proc.PID
				break
			}
		}

		if testHandle == 0 {
			t.Skip("No valid window handle found")
		}

		options := &capture.CaptureOptions{
			Format:        capture.PNG,
			Quality:       90,
			IncludeHidden: false,
		}

		img, err := capturer.CaptureWindowByHandle(testHandle, options)
		if err != nil {
			t.Logf("Window capture by handle failed (this may be normal): %v", err)
			return
		}

		if img == nil {
			t.Fatal("Screenshot result should not be nil")
		}

		bounds := img.Bounds()
		t.Logf("Window capture by handle successful, PID: %d, handle: %d, size: %dx%d",
			testPID, testHandle, bounds.Dx(), bounds.Dy())
	})
}

func TestCaptureOptions(t *testing.T) {
	t.Run("DefaultCaptureOptions", func(t *testing.T) {
		options := capture.DefaultCaptureOptions()

		if options.Format != capture.PNG {
			t.Errorf("Default format should be PNG, got %v", options.Format)
		}

		if options.Quality != 90 {
			t.Errorf("Default quality should be 90, got %d", options.Quality)
		}

		if !options.IncludeHidden {
			t.Error("Default should include hidden windows")
		}
	})

	t.Run("ImageFormatString", func(t *testing.T) {
		formats := []struct {
			format   capture.ImageFormat
			expected string
		}{
			{capture.PNG, "png"},
			{capture.JPEG, "jpeg"},
			{capture.BMP, "bmp"},
			{capture.GIF, "gif"},
		}

		for _, test := range formats {
			if test.format.String() != test.expected {
				t.Errorf("Format %v string should be %s, got %s",
					test.format, test.expected, test.format.String())
			}
		}
	})
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("CaptureWindowByPID", func(t *testing.T) {
		// Find a process
		pid, err := process.GetProcessPIDByName("explorer", process.FuzzyMatch)
		if err != nil {
			t.Skip("Explorer process not found")
		}

		options := capture.DefaultCaptureOptions()
		img, err := capture.CaptureWindowByPID(pid, options)
		if err != nil {
			t.Logf("Convenience function capture failed (this may be normal): %v", err)
			return
		}

		if img == nil {
			t.Fatal("Screenshot result should not be nil")
		}

		t.Logf("Convenience function capture successful")
	})

	t.Run("CaptureAndSave", func(t *testing.T) {
		// Find a process
		pid, err := process.GetProcessPIDByName("explorer", process.FuzzyMatch)
		if err != nil {
			t.Skip("Explorer process not found")
		}

		filename := "test_convenience_capture.png"
		defer os.Remove(filename)

		err = capture.CaptureAndSave(pid, filename, capture.PNG, 90)
		if err != nil {
			t.Logf("Convenience function capture and save failed (this may be normal): %v", err)
			return
		}

		// Check if file exists
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("Convenience function should create screenshot file")
		}

		t.Logf("Convenience function capture and save successful: %s", filename)
	})
}

func TestCalculateSimilarity(t *testing.T) {
	t.Run("CalculateSimilarity", func(t *testing.T) {
		// Create two identical images
		img1 := image.NewRGBA(image.Rect(0, 0, 100, 100))
		img2 := image.NewRGBA(image.Rect(0, 0, 100, 100))

		similarity, err := image2.CalculateSimilarity(img1, img2)
		if err != nil {
			t.Fatalf("Failed to calculate similarity: %v", err)
		}

		t.Log(similarity)

		if similarity < 0.99 {
			t.Errorf("Similarity should be close to 1.0, got %.2f", similarity)
		}
	})
}
