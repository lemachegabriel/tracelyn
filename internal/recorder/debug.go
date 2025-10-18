package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// debugLogFile is the package-level file handle for debug logging
var debugLogFile *os.File

// InitDebugLog initializes the debug log file
func InitDebugLog() error {
	dir, err := getTracelynDir()
	if err != nil {
		return err
	}

	logPath := filepath.Join(dir, "debug.log")
	debugLogFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create debug log: %w", err)
	}

	fmt.Fprintf(debugLogFile, "\n\n=== Recording session started at %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}

func CloseDebugLog() {
	if debugLogFile != nil {
		debugLogFile.Close()
		debugLogFile = nil
	}
}

// Only writes if InitDebugLog() was called successfully
func LogDebug(format string, args ...interface{}) {
	if debugLogFile != nil {
		fmt.Fprintf(debugLogFile, format+"\n", args...)
		debugLogFile.Sync()
	}
}

// Example usage in StartRecording():
//
// func StartRecording() error {
// 	// Initialize debug logging
// 	if err := InitDebugLog(); err != nil {
// 		return err
// 	}
// 	defer CloseDebugLog()
//
// 	// ... rest of code ...
// 	LogDebug("Some debug message: %v", value)
// }
