package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// createSessionFile creates a new session file with timestamp-based naming
// Returns the file handle, filename, and any error
func createSessionFile() (*os.File, string, error) {
	// Ensure sessions directory exists
	sessionsDir := "sessions"
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, "", fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Generate timestamp-based filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("session_%s.txt", timestamp)
	filepath := filepath.Join(sessionsDir, filename)

	// Create the session file
	file, err := os.Create(filepath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create session file: %w", err)
	}

	return file, filepath, nil
}

// setupShellCommand creates a transparent sub-shell command using the user's default shell
// that inherits the current environment and working directory
func setupShellCommand() (*exec.Cmd, error) {
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

	// Set working directory to preserve user's location
	cmd.Dir = cwd

	return cmd, nil
}

// startPTY starts the command with a PTY and returns the PTY master file handle
func startPTY(cmd *exec.Cmd) (*os.File, error) {
	// Start the command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	return ptmx, nil
}

// configureTerminal sets the terminal to raw mode and inherits the terminal size
// Returns the original terminal state for restoration on exit
func configureTerminal(ptmx *os.File) (*term.State, error) {
	// Set stdin to raw mode to pass all input directly to PTY
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}

	// Copy current terminal size to PTY to ensure proper display
	if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
		// Restore terminal state before returning error
		term.Restore(int(os.Stdin.Fd()), oldState)
		return nil, fmt.Errorf("failed to inherit terminal size: %w", err)
	}

	return oldState, nil
}

// StartRecording starts a new recording session
// This function will be implemented in later steps
func StartRecording() error {
	// TODO: Implement in Step 10
	return nil
}

// StopRecording stops the current recording session
// This function will be implemented in later steps
func StopRecording() error {
	// TODO: Implement in Step 11
	return nil
}
