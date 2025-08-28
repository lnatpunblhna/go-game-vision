//go:build darwin

package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// DarwinProcessManager macOS platform process manager
type DarwinProcessManager struct{}

// newPlatformProcessManager creates platform-specific process manager
func newPlatformProcessManager() ProcessManager {
	return &DarwinProcessManager{}
}

// GetProcessByName gets process information by process name
func (d *DarwinProcessManager) GetProcessByName(name string, mode MatchMode) ([]ProcessInfo, error) {
	allProcesses, err := d.ListAllProcesses()
	if err != nil {
		return nil, utils.WrapError(err, "failed to list processes")
	}

	var result []ProcessInfo
	for _, proc := range allProcesses {
		match := false
		switch mode {
		case ExactMatch:
			match = proc.Name == name
		case FuzzyMatch:
			// 在名称和路径中都进行模糊匹配
			nameLower := strings.ToLower(proc.Name)
			pathLower := strings.ToLower(proc.Path)
			searchLower := strings.ToLower(name)
			match = strings.Contains(nameLower, searchLower) ||
				strings.Contains(pathLower, searchLower)
		}

		if match {
			result = append(result, proc)
		}
	}

	utils.Debug("Found %d matching processes: %s", len(result), name)
	return result, nil
}

// GetProcessByPID gets process information by PID
func (d *DarwinProcessManager) GetProcessByPID(pid uint32) (*ProcessInfo, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "pid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		return nil, utils.WrapError(err, "failed to execute ps command")
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, utils.ErrProcessNotFound
	}

	// Parse ps output
	fields := strings.Fields(lines[1])
	if len(fields) < 3 {
		return nil, utils.ErrProcessNotFound
	}

	parsedPID, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		return nil, utils.WrapError(err, "failed to parse PID")
	}

	processName := fields[1]
	processPath := ""
	if len(fields) > 2 {
		processPath = strings.Join(fields[2:], " ")
	}

	return &ProcessInfo{
		PID:  uint32(parsedPID),
		Name: processName,
		Path: processPath,
	}, nil
}

// ListAllProcesses lists all processes
func (d *DarwinProcessManager) ListAllProcesses() ([]ProcessInfo, error) {
	cmd := exec.Command("ps", "-eo", "pid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		return nil, utils.WrapError(err, "failed to execute ps command")
	}

	lines := strings.Split(string(output), "\n")
	var processes []ProcessInfo

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			continue
		}

		processName := fields[1]
		processPath := ""
		if len(fields) > 2 {
			processPath = strings.Join(fields[2:], " ")
		}

		processes = append(processes, ProcessInfo{
			PID:  uint32(pid),
			Name: processName,
			Path: processPath,
		})
	}

	utils.Debug("Listed %d processes", len(processes))
	return processes, nil
}

// IsProcessRunning checks if process is running
func (d *DarwinProcessManager) IsProcessRunning(pid uint32) bool {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid))
	err := cmd.Run()
	return err == nil
}
