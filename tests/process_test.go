package tests

import (
	"testing"

	"github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func TestProcessManager(t *testing.T) {
	manager := process.NewProcessManager()

	t.Run("ListAllProcesses", func(t *testing.T) {
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(processes) == 0 {
			t.Error("Should have at least one process")
		}

		t.Logf("Found %d processes", len(processes))

		// Check basic information of first process
		if len(processes) > 0 {
			proc := processes[0]
			// In Windows, system process PID may be 0, which is normal
			if proc.Name == "" {
				t.Error("Process name should not be empty")
			}
			t.Logf("First process: PID=%d, Name=%s", proc.PID, proc.Name)
		}
	})

	t.Run("GetProcessByPID", func(t *testing.T) {
		// 获取所有进程
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("列出进程失败: %v", err)
		}

		if len(processes) == 0 {
			t.Skip("没有进程可测试")
		}

		// 测试获取第一个进程
		testPID := processes[0].PID
		proc, err := manager.GetProcessByPID(testPID)
		if err != nil {
			t.Fatalf("根据PID获取进程失败: %v", err)
		}

		if proc.PID != testPID {
			t.Errorf("期望PID %d, 得到 %d", testPID, proc.PID)
		}

		t.Logf("Successfully got process: PID=%d, Name=%s", proc.PID, proc.Name)
	})

	t.Run("IsProcessRunning", func(t *testing.T) {
		// 获取所有进程
		processes, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("列出进程失败: %v", err)
		}

		if len(processes) == 0 {
			t.Skip("没有进程可测试")
		}

		// Test an existing process (skip system processes with PID 0-10)
		var testPID uint32
		for _, proc := range processes {
			if proc.PID > 10 && proc.Name != "" {
				testPID = proc.PID
				break
			}
		}

		if testPID > 0 {
			isRunning := manager.IsProcessRunning(testPID)
			if !isRunning {
				t.Errorf("Process %d should be running", testPID)
			}
		} else {
			t.Skip("No suitable process found for testing")
		}

		// Test a non-existent process
		isRunning := manager.IsProcessRunning(99999)
		if isRunning {
			t.Error("Non-existent process should not show as running")
		}

		t.Logf("Process running status check passed")
	})

	t.Run("GetProcessByName", func(t *testing.T) {
		// Get all processes
		allProcesses, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(allProcesses) == 0 {
			t.Skip("No processes available for testing")
		}

		// Select a process name for testing
		testProcessName := allProcesses[0].Name

		// Exact match test
		processes, err := manager.GetProcessByName(testProcessName, process.ExactMatch)
		if err != nil {
			t.Fatalf("Failed to get process by name: %v", err)
		}

		found := false
		for _, proc := range processes {
			if proc.Name == testProcessName {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Exact match should find process %s", testProcessName)
		}

		t.Logf("Exact match found %d processes: %s", len(processes), testProcessName)

		// Fuzzy match test (using part of process name)
		if len(testProcessName) > 3 {
			partialName := testProcessName[:3]
			processes, err = manager.GetProcessByName(partialName, process.FuzzyMatch)
			if err != nil {
				t.Fatalf("Fuzzy match failed: %v", err)
			}

			t.Logf("Fuzzy match '%s' found %d processes", partialName, len(processes))
		}
	})
}

func TestProcessConvenienceFunctions(t *testing.T) {
	t.Run("GetProcessPIDByName", func(t *testing.T) {
		// Get all processes
		manager := process.NewProcessManager()
		allProcesses, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(allProcesses) == 0 {
			t.Skip("No processes available for testing")
		}

		// Select a process for testing
		testProcessName := allProcesses[0].Name
		expectedPID := allProcesses[0].PID

		// Test convenience function
		pid, err := process.GetProcessPIDByName(testProcessName, process.ExactMatch)
		if err != nil {
			t.Fatalf("Convenience function failed to get PID: %v", err)
		}

		if pid != expectedPID {
			t.Errorf("Expected PID %d, got %d", expectedPID, pid)
		}

		t.Logf("Convenience function successfully got PID: %d", pid)
	})

	t.Run("GetAllProcessPIDsByName", func(t *testing.T) {
		// Get all processes
		manager := process.NewProcessManager()
		allProcesses, err := manager.ListAllProcesses()
		if err != nil {
			t.Fatalf("Failed to list processes: %v", err)
		}

		if len(allProcesses) == 0 {
			t.Skip("No processes available for testing")
		}

		// Select a process for testing
		testProcessName := allProcesses[0].Name

		// Test convenience function
		pids, err := process.GetAllProcessPIDsByName(testProcessName, process.ExactMatch)
		if err != nil {
			t.Fatalf("Convenience function failed to get all PIDs: %v", err)
		}

		if len(pids) == 0 {
			t.Error("Should find at least one PID")
		}

		t.Logf("Convenience function found %d PIDs", len(pids))
	})

	t.Run("NonExistentProcess", func(t *testing.T) {
		// Test non-existent process
		_, err := process.GetProcessPIDByName("non_existent_process_12345", process.ExactMatch)
		if err == nil {
			t.Error("Looking for non-existent process should return error")
		}

		t.Logf("Correctly handled non-existent process: %v", err)
	})
}
