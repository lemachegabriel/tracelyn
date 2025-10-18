package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/term"
)

// CreateSessionFile creates a new session file with timestamp-based naming
// Returns the file handle, filename, and any error
func CreateSessionFile() (*os.File, string, error) {
	// Ensure tracelyn directories exist
	if err := ensureTracelynDirs(); err != nil {
		return nil, "", err
	}

	// Get sessions directory path
	sessionsDir, err := getSessionsDir()
	if err != nil {
		return nil, "", err
	}

	// Generate timestamp-based filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("session_%s.txt", timestamp)
	filePath := filepath.Join(sessionsDir, filename)

	// Create the session file
	file, err := os.Create(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create session file: %w", err)
	}

	return file, filePath, nil
}

// SetupShellCommand creates a transparent sub-shell command using the user's default shell
// that inherits the current environment and working directory
// Sets TRACELYN_SESSION_ID environment variable to identify the recording session
func SetupShellCommand(sessionID string) (*exec.Cmd, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Get user's default shell from SHELL environment variable
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // fallback to bash if SHELL is not set
	}

	// Create shell command with -l (login) flag to load user's configuration
	cmd := exec.Command(shell, "-l")

	// Copy all environment variables to make sub-shell transparent
	cmd.Env = os.Environ()

	// Add TRACELYN_SESSION_ID to identify this recording session
	cmd.Env = append(cmd.Env, fmt.Sprintf("TRACELYN_SESSION_ID=%s", sessionID))

	// Set working directory to preserve user's location
	cmd.Dir = cwd

	return cmd, nil
}

// HandleShutdownSignals sets up signal handlers for graceful shutdown
// Returns a channel that receives SIGTERM and SIGINT signals
func HandleShutdownSignals() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	return sigChan
}

// StartRecording starts a new recording session
func StartRecording() error {
	// Load session registry
	registry, err := LoadSessions()
	if err != nil {
		return err
	}

	// Cleanup stale sessions
	registry.CleanupStaleSessions()

	// Create session file
	sessionFile, sessionPath, err := CreateSessionFile()
	if err != nil {
		return err
	}
	defer sessionFile.Close()

	// Register session in registry with current process PID (the tracelyn record process)
	session := registry.CreateSession(
		filepath.Base(sessionPath),
		sessionPath,
		os.Getpid(), // PID of the tracelyn record process
	)
	if err := registry.Save(); err != nil {
		return err
	}

	// Ensure cleanup on exit
	defer func() {
		// Reload registry to get latest state (avoid race condition)
		currentRegistry, err := LoadSessions()
		if err != nil {
			return
		}
		currentRegistry.CompleteSession(session.ID)
		currentRegistry.Save()
	}()

	// Setup shell command with session ID in environment
	cmd, err := SetupShellCommand(session.ID)
	if err != nil {
		return err
	}

	// Start PTY
	ptmx, err := StartPTY(cmd)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	// Configure terminal
	oldState, err := ConfigureTerminal(ptmx)
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Setup I/O copy with recording (returns resize callback)
	onResize, err := SetupIOCopy(ptmx, sessionFile)
	if err != nil {
		return err
	}

	// Handle window resize (pass resize callback from SetupIOCopy)
	resizeCh := HandleResize(ptmx, onResize)
	defer signal.Stop(resizeCh)

	// Handle shutdown signals (SIGTERM, SIGINT)
	sigChan := HandleShutdownSignals()
	defer signal.Stop(sigChan)

	// Wait for shell to exit or shutdown signal
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-sigChan:
		// Received shutdown signal (SIGTERM or SIGINT)
		// Terminal will be restored by defer, session removed, file closed
		fmt.Printf("\nRecording stopped. Session saved to: %s\n", sessionPath)
		return nil
	case err := <-done:
		// Shell exited normally
		if err != nil {
			// Ignore exit errors (user may exit with Ctrl+D or 'exit')
		}
		fmt.Printf("\nRecording saved to: %s\n", sessionPath)
		return nil
	}
}

// StopSessionByID stops a specific recording session by ID
func StopSessionByID(sessionID string) error {
	registry, err := LoadSessions()
	if err != nil {
		return err
	}

	session, err := registry.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if !IsProcessRunning(session.PID) {
		registry.CompleteSession(sessionID)
		registry.Save()
		return fmt.Errorf("session process is not running")
	}

	process, err := os.FindProcess(session.PID)
	if err != nil {
		return fmt.Errorf("failed to find session process: %w", err)
	}

	return process.Signal(syscall.SIGTERM)
}

// StopCurrentSession stops the recording session in the current terminal
// Returns the session ID that was stopped
func StopCurrentSession() (string, error) {
	// Check if TRACELYN_SESSION_ID environment variable is set
	sessionID := os.Getenv("TRACELYN_SESSION_ID")
	if sessionID == "" {
		return "", fmt.Errorf("no active session in current terminal")
	}

	// Stop the session by ID
	if err := StopSessionByID(sessionID); err != nil {
		return "", err
	}

	return sessionID, nil
}
