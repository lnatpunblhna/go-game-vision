package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/image"
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
			fmt.Println("Usage: go-game-vision capture <process_name> [output_file]")
			return
		}
		processName := os.Args[2]
		outputFile := "capture.png"
		windowTitle := ""
		if len(os.Args) >= 4 {
			outputFile = os.Args[3]
		}
		if len(os.Args) >= 5 {
			windowTitle = os.Args[4]
		}
		captureWindow(processName, outputFile, windowTitle)

	case "compare":
		handleCompareCommand()
	case "help", "--help", "-h":
		showUsage()
	case "version", "--version", "-v":
		showVersion()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showUsage()
	}
}

func showUsage() {
	fmt.Println("Go Game Vision - Cross-platform Window Capture and Image Processing Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go-game-vision <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  list                                    - List all processes")
	fmt.Println("  capture <process_name> [output_file]    - Capture specified process window")
	fmt.Println("  compare <image1> <image2> [options]     - Compare two images")
	fmt.Println("  help, --help, -h                       - Show help information")
	fmt.Println("  version, --version, -v                 - Show version information")
	fmt.Println()
	fmt.Println("Compare Options:")
	fmt.Println("  --method <method>     Comparison method (template, feature, histogram, similarity)")
	fmt.Println("  --threshold <value>   Similarity threshold (0.0-1.0)")
	fmt.Println("  --output <file>       Save comparison result to file")
	fmt.Println("  --verbose             Show detailed information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go-game-vision list")
	fmt.Println("  go-game-vision capture notepad screenshot.png")
	fmt.Println("  go-game-vision compare image1.png image2.png --method template")
	fmt.Println("  go-game-vision compare img1.jpg img2.jpg --method feature --threshold 0.8")
	fmt.Println()
}

func showVersion() {
	fmt.Println("Go Game Vision v1.0.0")
	fmt.Println("Cross-platform Window Capture and Image Processing Tool")
	fmt.Println("Supported Platforms: Windows, macOS, Linux")
	fmt.Println()
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

func captureWindow(processName, outputFile, windowTitle string) {
	fmt.Printf("Searching for process: %s\n", processName)

	// Find process
	pid, err := process.GetProcessPIDByName(processName, process.FuzzyMatch)
	if err != nil {
		log.Fatalf("Failed to find process: %v", err)
	}

	fmt.Printf("Found process %s, PID: %d\n", processName, pid)

	// Create capturer
	capturer := capture.NewScreenCapture()

	// Capture options
	options := &capture.CaptureOptions{
		Format:        capture.PNG,
		Quality:       90,
		IncludeHidden: true,
		WindowTitle:   windowTitle,
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

func handleCompareCommand() {
	if len(os.Args) < 4 {
		fmt.Println("Error: Please specify two image paths")
		fmt.Println("Usage: go-game-vision compare <image1> <image2> [options]")
		fmt.Println("Use 'go-game-vision help' for more information")
		return
	}

	image1Path := os.Args[2]
	image2Path := os.Args[3]

	// 解析选项
	var method = image.TemplateMatching
	var threshold = 0.5
	var comparer = image.NewImageComparer(method)
	var outputFile string
	var verbose bool

	for i := 4; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--method":
			if i+1 < len(os.Args) {
				method = comparer.ParseCompareMethod(os.Args[i+1])
				i++
			}
		case "--threshold":
			if i+1 < len(os.Args) {
				if t, err := strconv.ParseFloat(os.Args[i+1], 64); err == nil {
					threshold = t
				}
				i++
			}
		case "--output":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "--verbose":
			verbose = true
		}
	}

	// 检查文件是否存在
	if _, err := os.Stat(image1Path); os.IsNotExist(err) {
		fmt.Printf("Error: Image file '%s' does not exist\n", image1Path)
		return
	}
	if _, err := os.Stat(image2Path); os.IsNotExist(err) {
		fmt.Printf("Error: Image file '%s' does not exist\n", image2Path)
		return
	}

	fmt.Printf("Comparing images:\n")
	fmt.Printf("  Image 1: %s\n", image1Path)
	fmt.Printf("  Image 2: %s\n", image2Path)

	if verbose {
		fmt.Printf("  Method: %s\n", comparer.GetMethodName(method))
		fmt.Printf("  Threshold: %.2f\n", threshold)
		if outputFile != "" {
			fmt.Printf("  Output file: %s\n", outputFile)
		}
	}

	// 加载图像
	img1, err := comparer.LoadImage(image1Path)
	if err != nil {
		log.Fatalf("Failed to load image 1: %v", err)
	}

	img2, err := comparer.LoadImage(image2Path)
	if err != nil {
		log.Fatalf("Failed to load image 2: %v", err)
	}

	// 执行对比
	result, err := comparer.CompareImages(img1, img2)
	if err != nil {
		log.Fatalf("Image comparison failed: %v", err)
	}

	// 显示结果
	fmt.Println("\n=== Comparison Result ===")
	fmt.Printf("Similarity: %.4f (%.2f%%)\n", result.Similarity, result.Similarity*100)
	fmt.Printf("Confidence: %.4f\n", result.Confidence)
	fmt.Printf("Method: %s\n", comparer.GetMethodName(result.Method))

	if result.Location.X != 0 || result.Location.Y != 0 {
		fmt.Printf("Match location: (%d, %d)\n", result.Location.X, result.Location.Y)
	}

	// 判断是否匹配
	isMatch := result.Similarity >= threshold
	fmt.Printf("Match (threshold %.2f): %v\n", threshold, isMatch)

	if verbose {
		fmt.Printf("\nDetailed Information:\n")
		fmt.Printf("  Image 1 size: %dx%d\n", img1.Bounds().Dx(), img1.Bounds().Dy())
		fmt.Printf("  Image 2 size: %dx%d\n", img2.Bounds().Dx(), img2.Bounds().Dy())
	}

	// 保存结果到文件
	if outputFile != "" {
		resultText := fmt.Sprintf("Image Comparison Result\n")
		resultText += fmt.Sprintf("Image 1: %s\n", image1Path)
		resultText += fmt.Sprintf("Image 2: %s\n", image2Path)
		resultText += fmt.Sprintf("Method: %s\n", comparer.GetMethodName(result.Method))
		resultText += fmt.Sprintf("Similarity: %.4f (%.2f%%)\n", result.Similarity, result.Similarity*100)
		resultText += fmt.Sprintf("Confidence: %.4f\n", result.Confidence)
		resultText += fmt.Sprintf("Threshold: %.2f\n", threshold)
		resultText += fmt.Sprintf("Match: %v\n", isMatch)
		if result.Location.X != 0 || result.Location.Y != 0 {
			resultText += fmt.Sprintf("Match location: (%d, %d)\n", result.Location.X, result.Location.Y)
		}

		err = os.WriteFile(outputFile, []byte(resultText), 0644)
		if err != nil {
			fmt.Printf("Warning: Failed to save result to file: %v\n", err)
		} else {
			fmt.Printf("Result saved to: %s\n", outputFile)
		}
	}
}
