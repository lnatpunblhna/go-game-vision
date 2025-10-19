//go:build windows

package process

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"golang.org/x/sys/windows"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	psapi                        = windows.NewLazySystemDLL("psapi.dll")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = kernel32.NewProc("Process32FirstW")
	procProcess32Next            = kernel32.NewProc("Process32NextW")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procGetModuleFileNameEx      = psapi.NewProc("GetModuleFileNameExW")
)

// Windows process constants
const (
	TH32CS_SNAPPROCESS        = 0x00000002 // Include all processes in the snapshot
	PROCESS_QUERY_INFORMATION = 0x0400     // Required to retrieve certain process information
	PROCESS_VM_READ           = 0x0010     // Required to read memory using ReadProcessMemory
	MAX_PATH                  = 260        // Maximum path length in Windows
)

// PROCESSENTRY32 describes an entry from a list of the processes residing in the system address space
type PROCESSENTRY32 struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [MAX_PATH]uint16
}

// WindowsProcessManager Windows platform process manager
type WindowsProcessManager struct{}

// newPlatformProcessManager creates platform-specific process manager
func newPlatformProcessManager() ProcessManager {
	return &WindowsProcessManager{}
}

// GetProcessByName gets process information by process name
func (w *WindowsProcessManager) GetProcessByName(name string, mode MatchMode) ([]ProcessInfo, error) {
	processes, err := w.ListAllProcesses()
	if err != nil {
		return nil, utils.WrapError(err, "failed to list all processes")
	}

	var result []ProcessInfo
	for _, proc := range processes {
		var match bool
		switch mode {
		case ExactMatch:
			match = strings.EqualFold(proc.Name, name) || strings.EqualFold(proc.Name, name+".exe")
		case FuzzyMatch:
			match = strings.Contains(strings.ToLower(proc.Name), strings.ToLower(name))
		}

		if match {
			result = append(result, proc)
		}
	}

	utils.Debug("Found %d matching processes: %s", len(result), name)
	return result, nil
}

// GetProcessByPID gets process information by PID
func (w *WindowsProcessManager) GetProcessByPID(pid uint32) (*ProcessInfo, error) {
	processes, err := w.ListAllProcesses()
	if err != nil {
		return nil, utils.WrapError(err, "failed to list all processes")
	}

	for _, proc := range processes {
		if proc.PID == pid {
			return &proc, nil
		}
	}

	return nil, utils.ErrProcessNotFound
}

// ListAllProcesses lists all processes
func (w *WindowsProcessManager) ListAllProcesses() ([]ProcessInfo, error) {
	snapshot, _, err := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if snapshot == uintptr(syscall.InvalidHandle) {
		return nil, utils.WrapError(err, "failed to create process snapshot")
	}
	defer windows.CloseHandle(windows.Handle(snapshot))

	var processes []ProcessInfo
	var pe32 PROCESSENTRY32
	pe32.dwSize = uint32(unsafe.Sizeof(pe32))

	ret, _, _ := procProcess32First.Call(snapshot, uintptr(unsafe.Pointer(&pe32)))
	if ret == 0 {
		return nil, fmt.Errorf("failed to get first process")
	}

	for {
		processName := windows.UTF16ToString(pe32.szExeFile[:])
		processPath := w.getProcessPath(pe32.th32ProcessID)

		processes = append(processes, ProcessInfo{
			PID:  pe32.th32ProcessID,
			Name: processName,
			Path: processPath,
		})

		ret, _, _ := procProcess32Next.Call(snapshot, uintptr(unsafe.Pointer(&pe32)))
		if ret == 0 {
			break
		}
	}

	utils.Debug("Listed %d processes", len(processes))
	return processes, nil
}

// IsProcessRunning checks if process is running
func (w *WindowsProcessManager) IsProcessRunning(pid uint32) bool {
	handle, _, _ := procOpenProcess.Call(PROCESS_QUERY_INFORMATION, 0, uintptr(pid))
	if handle == 0 {
		return false
	}
	defer windows.CloseHandle(windows.Handle(handle))
	return true
}

// getProcessPath gets process path
func (w *WindowsProcessManager) getProcessPath(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer windows.CloseHandle(windows.Handle(handle))

	var path [MAX_PATH]uint16
	ret, _, _ := procGetModuleFileNameEx.Call(handle, 0, uintptr(unsafe.Pointer(&path[0])), MAX_PATH)
	if ret == 0 {
		return ""
	}

	return windows.UTF16ToString(path[:])
}
