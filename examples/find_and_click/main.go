// Command find_and_click demonstrates the end-to-end flow of go-game-vision:
// locate a process by name, capture its window, find a template image inside
// the screenshot with multi-scale matching, and click the match — only when the
// similarity clears a threshold.
//
// Usage:
//
//	go run ./examples/find_and_click -process nikke.exe -template ./button.png -threshold 0.8
//
// Requires OpenCV to be installed (see the project README).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	ggvimage "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/process"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func main() {
	processName := flag.String("process", "", "target process name (e.g. nikke.exe)")
	templatePath := flag.String("template", "", "path to the template image to find")
	threshold := flag.Float64("threshold", ggvimage.DefaultMatchThreshold, "minimum similarity (0-1) required to click")
	dryRun := flag.Bool("dry-run", false, "find only, do not click")
	flag.Parse()

	if *processName == "" || *templatePath == "" {
		flag.Usage()
		os.Exit(2)
	}

	// 1. Resolve the process PID by name (exact match).
	pid, err := process.GetProcessPIDByName(*processName, process.ExactMatch)
	if err != nil {
		fatalf("find process %q: %v", *processName, err)
	}
	fmt.Printf("process %q -> PID %d\n", *processName, pid)

	// 2. Grab the window geometry and a screenshot of it.
	windowInfo, err := capture.GetWindowInfoByPID(pid)
	if err != nil {
		fatalf("get window info: %v", err)
	}
	screenshot, err := capture.CaptureWindowByPID(pid, nil)
	if err != nil {
		fatalf("capture window: %v", err)
	}

	// 3. Load the template and run multi-scale template matching.
	template, err := ggvimage.LoadImage(*templatePath)
	if err != nil {
		fatalf("load template: %v", err)
	}
	cfg := ggvimage.DefaultMultiScaleConfig()
	cfg.Threshold = *threshold
	result, err := ggvimage.MultiScaleTemplateMatch(screenshot, template, cfg)
	if err != nil {
		fatalf("match: %v", err)
	}

	if result.Similarity < *threshold {
		fatalf("no match: best similarity %.4f < threshold %.2f", result.Similarity, *threshold)
	}
	screen := result.ToScreenCoordinates(windowInfo)
	fmt.Printf("match: similarity=%.4f scale=%.2f window=(%d,%d) screen=(%d,%d)\n",
		result.Similarity, result.Scale, result.Location.X, result.Location.Y, screen.X, screen.Y)

	if *dryRun {
		fmt.Println("dry-run: skipping click")
		return
	}

	// 4. Click at the match (screen coordinates, with human-like randomization).
	err = result.ClickAtMatch(windowInfo, &mouse.ClickOptions{
		Button:       mouse.LeftButton,
		Delay:        50,
		RandomDelay:  true,
		RestoreFocus: true,
	})
	if err != nil {
		fatalf("click: %v", err)
	}
	fmt.Println("clicked successfully")
}

func fatalf(format string, args ...interface{}) {
	utils.Error(format, args...)
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
