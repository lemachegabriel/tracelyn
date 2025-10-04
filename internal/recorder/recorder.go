package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"golang.org/x/term"
)

// CreateSessionFile creates a new session file with timestamp-based naming
// Returns the file handle, filename, and any error
func CreateSessionFile() (*os.File, string, error) {
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

// SetupShellCommand creates a transparent sub-shell command using the user's default shell
// that inherits the current environment and working directory
func SetupShellCommand() (*exec.Cmd, error) {
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

// StartPTY starts the command with a PTY and returns the PTY master file handle
func StartPTY(cmd *exec.Cmd) (*os.File, error) {
	// Start the command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	return ptmx, nil
}

// ConfigureTerminal sets the terminal to raw mode and inherits the terminal size
// Returns the original terminal state for restoration on exit
func ConfigureTerminal(ptmx *os.File) (*term.State, error) {
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

// HandleResize listens for SIGWINCH (window resize) signals and updates the PTY size accordingly
// Returns the signal channel for cleanup
func HandleResize(ptmx *os.File) chan os.Signal {
	// Create channel to receive window resize signals
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)

	// Start goroutine to handle resize events
	go func() {
		for range resizeCh {
			// Update PTY size to match current terminal size
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				// Silently ignore resize errors to avoid disrupting the session
				// The terminal will continue working, just with the old size
				continue
			}
		}
	}()

	return resizeCh
}

// captureState represents the current state of the recording session
type captureState int

const (
	waitingForEnter  captureState = iota // Waiting for user to press Enter
	waitingForOutput                     // Waiting for command output to complete
)

// extractLine gets a single line from the emulator, trimmed
func extractLine(emulator *vt.Emulator, y int) string {
	var line string
	width := emulator.Width()

	for x := 0; x < width; x++ {
		cell := emulator.CellAt(x, y)
		if cell != nil && cell.Content != "" {
			line += cell.Content
		}
	}

	return trimRight(line)
}

// saveNewContent extracts and saves only new lines from the virtual terminal
// Uses line-by-line tracking to avoid duplicates
func saveNewContent(emulator *vt.Emulator, file *os.File, savedLines *[]string) error {
	height := emulator.Height()
	var currentLines []string

	// Extract all current non-empty lines
	for y := 0; y < height; y++ {
		line := extractLine(emulator, y)
		if len(line) > 0 {
			currentLines = append(currentLines, line)
		}
	}

	// Find and save only new lines
	savedCount := len(*savedLines)
	for i := savedCount; i < len(currentLines); i++ {
		if _, err := file.WriteString(currentLines[i] + "\n"); err != nil {
			return fmt.Errorf("failed to write to session file: %w", err)
		}
	}

	// Update saved lines
	*savedLines = currentLines

	// Flush to disk
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync session file: %w", err)
	}

	return nil
}

// trimRight removes trailing whitespace from string
func trimRight(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[:end]
}

// SetupIOCopy sets up bidirectional I/O copying between the terminal and PTY
// with state-based saving: saves when user starts typing after command output
func SetupIOCopy(ptmx *os.File, sessionFile *os.File) error {
	// Get current terminal size for virtual terminal
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to standard 80x24 if we can't get size
		width, height = 80, 24
	}

	// Create virtual terminal emulator
	emulator := vt.NewEmulator(width, height)

	// Track state and saved lines
	state := waitingForEnter
	var savedLines []string

	// Goroutine 1: Read from PTY → write to stdout + feed to emulator
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
				emulator.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Goroutine 2: Read from stdin → detect state transitions + write to PTY
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}

			if n > 0 {
				// Write input to PTY first
				ptmx.Write(buf[:n])

				// Small delay to let the command line appear in the buffer
				time.Sleep(10 * time.Millisecond)

				// Detect Enter key or typing during output
				for i := 0; i < n; i++ {
					if buf[i] == '\r' || buf[i] == '\n' {
						// Enter pressed → save command line + transition to waitingForOutput
						saveErr := saveNewContent(emulator, sessionFile, &savedLines)
						if saveErr != nil {
							// Silently handle errors
						}
						state = waitingForOutput
						break
					} else if state == waitingForOutput {
						// Any key pressed while waiting for output → save output + transition
						saveErr := saveNewContent(emulator, sessionFile, &savedLines)
						if saveErr != nil {
							// Silently handle errors
						}
						state = waitingForEnter
						break
					}
				}
			}
		}
	}()

	return nil
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
