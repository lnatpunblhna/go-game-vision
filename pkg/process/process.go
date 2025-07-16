package process

import (
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

// ProcessInfo process information struct
type ProcessInfo struct {
	PID  uint32 // Process ID
	Name string // Process name
	Path string // Process path
}

// MatchMode matching mode
type MatchMode int

const (
	ExactMatch MatchMode = iota // Exact matching
	FuzzyMatch                  // Fuzzy matching
)

// ProcessManager process manager interface
type ProcessManager interface {
	// GetProcessByName gets process information by process name
	GetProcessByName(name string, mode MatchMode) ([]ProcessInfo, error)

	// GetProcessByPID gets process information by PID
	GetProcessByPID(pid uint32) (*ProcessInfo, error)

	// ListAllProcesses lists all processes
	ListAllProcesses() ([]ProcessInfo, error)

	// IsProcessRunning checks if process is running
	IsProcessRunning(pid uint32) bool
}

// NewProcessManager creates a process manager instance
func NewProcessManager() ProcessManager {
	return newPlatformProcessManager()
}

// GetProcessPIDByName convenience function: get the first matching PID by process name
func GetProcessPIDByName(name string, mode MatchMode) (uint32, error) {
	manager := NewProcessManager()
	processes, err := manager.GetProcessByName(name, mode)
	if err != nil {
		return 0, utils.WrapError(err, "failed to get process")
	}

	if len(processes) == 0 {
		utils.Warn("Process not found: %s", name)
		return 0, utils.ErrProcessNotFound
	}

	if len(processes) > 1 {
		utils.Info("Found multiple processes %s, returning first PID: %d", name, processes[0].PID)
	}

	return processes[0].PID, nil
}

// GetAllProcessPIDsByName convenience function: get all matching PIDs by process name
func GetAllProcessPIDsByName(name string, mode MatchMode) ([]uint32, error) {
	manager := NewProcessManager()
	processes, err := manager.GetProcessByName(name, mode)
	if err != nil {
		return nil, utils.WrapError(err, "failed to get process")
	}

	if len(processes) == 0 {
		utils.Warn("Process not found: %s", name)
		return nil, utils.ErrProcessNotFound
	}

	pids := make([]uint32, len(processes))
	for i, proc := range processes {
		pids[i] = proc.PID
	}

	utils.Info("Found %d processes %s", len(pids), name)
	return pids, nil
}
