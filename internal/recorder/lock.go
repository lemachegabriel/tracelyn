package recorder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const lockFilePath = ".tracelyn.lock"

// CreateLockFile creates a lock file with the given PID
func CreateLockFile(pid int) error {
	content := fmt.Sprintf("%d", pid)
	return os.WriteFile(lockFilePath, []byte(content), 0644)
}

// ReadLockFile reads the PID from the lock file
func ReadLockFile() (int, error) {
	data, err := os.ReadFile(lockFilePath)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in lock file: %v", err)
	}

	return pid, nil
}

// RemoveLockFile removes the lock file
func RemoveLockFile() error {
	err := os.Remove(lockFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists without actually sending a signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// CheckExistingRecording checks if there's an existing recording session
// Returns an error if a valid recording is already running
func CheckExistingRecording() error {
	pid, err := ReadLockFile()
	if err != nil {
		if os.IsNotExist(err) {
			// No lock file exists, we're good to go
			return nil
		}
		return fmt.Errorf("error reading lock file: %v", err)
	}

	// Check if the process is still running
	if IsProcessRunning(pid) {
		return fmt.Errorf("recording already in progress (PID: %d)", pid)
	}

	// Stale lock file, remove it
	if err := RemoveLockFile(); err != nil {
		return fmt.Errorf("error removing stale lock file: %v", err)
	}

	return nil
}
