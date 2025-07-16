package main

import (
	"fmt"
	"log"
	"os"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/process"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func main() {
	// Set log level
	utils.GlobalLogger = utils.NewLogger(utils.INFO)

	fmt.Println("=== Go Game Vision - Cross-platform Window Capture Tool ===")
	fmt.Println("Version: 1.0.0")
	fmt.Println("Supported Platforms: Windows, macOS")
	fmt.Println()

	// Check command line arguments
	if len(os.Args) < 2 {
		showUsage()
		return
	}

	command := os.Args[1]
	switch command {
	case "list":
		listProcesses()
	case "capture":
		if len(os.Args) < 3 {
			fmt.Println("Error: Please specify process name")
			fmt.Println("Usage: go run main.go capture <process_name> [output_file]")
			return
		}
		processName := os.Args[2]
		outputFile := "capture.png"
		if len(os.Args) >= 4 {
			outputFile = os.Args[3]
		}
		captureWindow(processName, outputFile)
	case "help":
		showUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showUsage()
	}
}

func showUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run main.go list                           - List all processes")
	fmt.Println("  go run main.go capture <process_name> [output_file] - Capture specified process window")
	fmt.Println("  go run main.go help                           - Show help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run main.go list")
	fmt.Println("  go run main.go capture notepad")
	fmt.Println("  go run main.go capture explorer window.png")
	fmt.Println()
	fmt.Println("For more examples, see examples/basic_usage.go")
}

func listProcesses() {
	fmt.Println("Listing system processes...")

	manager := process.NewProcessManager()
	processes, err := manager.ListAllProcesses()
	if err != nil {
		log.Fatalf("Failed to list processes: %v", err)
	}

	fmt.Printf("Found %d processes:\n", len(processes))
	fmt.Println("PID\tProcess Name")
	fmt.Println("---\t------------")

	for _, proc := range processes {
		fmt.Printf("%d\t%s\n", proc.PID, proc.Name)
	}
}

func captureWindow(processName, outputFile string) {
	fmt.Printf("Searching for process: %s\n", processName)

	// Find process
	pid, err := process.GetProcessPIDByName(processName, process.FuzzyMatch)
	if err != nil {
		log.Fatalf("Failed to find process: %v", err)
	}

	fmt.Printf("Found process %s, PID: %d\n", processName, pid)

	// Create capturer
	capturer := capture.NewScreenCapture()

	// Get window information
	windows, err := capturer.GetWindowsByPID(pid)
	if err != nil {
		log.Fatalf("Failed to get window information: %v", err)
	}

	if len(windows) == 0 {
		log.Fatalf("Process %s (PID: %d) has no visible windows", processName, pid)
	}

	fmt.Printf("Found %d windows\n", len(windows))
	for i, win := range windows {
		if i >= 3 { // Only show first 3
			break
		}
		fmt.Printf("  Window %d: %s\n", i+1, win.Title)
	}

	// Capture options
	options := &capture.CaptureOptions{
		Format:        capture.PNG,
		Quality:       90,
		IncludeHidden: true,
	}

	fmt.Println("Capturing window...")

	// Capture window
	img, err := capturer.CaptureWindowByPID(pid, options)
	if err != nil {
		log.Fatalf("Failed to capture: %v", err)
	}

	// Save screenshot
	err = capturer.SaveImage(img, outputFile, capture.PNG, 90)
	if err != nil {
		log.Fatalf("Failed to save screenshot: %v", err)
	}

	fmt.Printf("Screenshot saved to: %s\n", outputFile)
	fmt.Printf("Image size: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
}
