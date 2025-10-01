package recorder

import (
	"os"
	"testing"
)

func TestLockFileManagement(t *testing.T) {
	// Cleanup any existing lock file before and after test
	defer RemoveLockFile()
	RemoveLockFile()

	t.Run("CreateAndReadLockFile", func(t *testing.T) {
		pid := os.Getpid()

		err := CreateLockFile(pid)
		if err != nil {
			t.Fatalf("Failed to create lock file: %v", err)
		}

		readPid, err := ReadLockFile()
		if err != nil {
			t.Fatalf("Failed to read lock file: %v", err)
		}

		if readPid != pid {
			t.Errorf("Expected PID %d, got %d", pid, readPid)
		}
	})

	t.Run("RemoveLockFile", func(t *testing.T) {
		err := RemoveLockFile()
		if err != nil {
			t.Fatalf("Failed to remove lock file: %v", err)
		}

		_, err = ReadLockFile()
		if err == nil {
			t.Error("Expected error reading removed lock file")
		}
	})

	t.Run("IsProcessRunning", func(t *testing.T) {
		// Test with current process (should be running)
		if !IsProcessRunning(os.Getpid()) {
			t.Error("Current process should be running")
		}

		// Test with non-existent PID
		if IsProcessRunning(999999) {
			t.Error("Non-existent process should not be running")
		}
	})

	t.Run("CheckExistingRecording_NoLock", func(t *testing.T) {
		RemoveLockFile()

		err := CheckExistingRecording()
		if err != nil {
			t.Errorf("Should not error when no lock exists: %v", err)
		}
	})

	t.Run("CheckExistingRecording_ActiveProcess", func(t *testing.T) {
		pid := os.Getpid()
		CreateLockFile(pid)

		err := CheckExistingRecording()
		if err == nil {
			t.Error("Should error when active recording exists")
		}

		RemoveLockFile()
	})

	t.Run("CheckExistingRecording_StaleLock", func(t *testing.T) {
		// Create lock with non-existent PID
		CreateLockFile(999999)

		err := CheckExistingRecording()
		if err != nil {
			t.Errorf("Should remove stale lock: %v", err)
		}

		// Verify lock was removed
		_, err = ReadLockFile()
		if err == nil {
			t.Error("Stale lock should have been removed")
		}
	})
}
