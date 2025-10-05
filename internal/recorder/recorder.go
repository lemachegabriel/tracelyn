package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
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
	paused                               // Paused during fullscreen apps (vim, less, etc)
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
// Uses a hash-based approach to track what has been saved
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

	// Build a set of already saved lines for O(1) lookup
	savedSet := make(map[string]bool)
	for _, line := range *savedLines {
		savedSet[line] = true
	}

	// Save only lines that haven't been saved yet (preserving order)
	var newLines []string
	for _, line := range currentLines {
		if !savedSet[line] {
			if _, err := file.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to write to session file: %w", err)
			}
			newLines = append(newLines, line)
			savedSet[line] = true // Mark as saved
		}
	}

	// Append new lines to saved lines
	*savedLines = append(*savedLines, newLines...)

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

// detectAlternateScreen checks if buffer contains alternate screen codes
// Returns: enterScreen (true if entering vim/less), exitScreen (true if exiting)
func detectAlternateScreen(buf []byte, n int) (enterScreen bool, exitScreen bool) {
	// Search for alternate screen sequences anywhere in buffer
	s := string(buf[:n])

	// Detect enter alternate screen: ESC[?1049h or ESC[?47h
	if containsSequence(s, "\x1b[?1049h") || containsSequence(s, "\x1b[?47h") ||
		containsSequence(s, "\x1b[?1047h") {
		return true, false
	}

	// Detect exit alternate screen: ESC[?1049l or ESC[?47l
	if containsSequence(s, "\x1b[?1049l") || containsSequence(s, "\x1b[?47l") ||
		containsSequence(s, "\x1b[?1047l") {
		return false, true
	}

	return false, false
}

// containsSequence checks if string contains the sequence anywhere
func containsSequence(s, seq string) bool {
	for i := 0; i <= len(s)-len(seq); i++ {
		if s[i:i+len(seq)] == seq {
			return true
		}
	}
	return false
}

// SetupIOCopy sets up bidirectional I/O copying between the terminal and PTY
// with Enter-based saving: only saves content when Enter is pressed
// Automatically pauses recording when fullscreen apps (vim, less) are detected
func SetupIOCopy(ptmx *os.File, sessionFile *os.File) error {
	// Get current terminal size for virtual terminal
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to standard 80x24 if we can't get size
		width, height = 80, 24
	}

	// Create virtual terminal emulator
	emulator := vt.NewEmulator(width, height)

	// Track state and saved lines (protected by mutex for goroutine safety)
	var stateMutex sync.Mutex
	state := waitingForEnter
	var savedLines []string

	// Goroutine 1: Read from PTY → write to stdout + feed emulator + detect fullscreen apps
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				// Always write to stdout (user sees everything)
				os.Stdout.Write(buf[:n])

				// Detect alternate screen buffer transitions
				enterScreen, exitScreen := detectAlternateScreen(buf, n)

				stateMutex.Lock()
				if enterScreen {
					// Entering fullscreen app → pause recording
					fmt.Fprintf(os.Stderr, "\r\n[tracelyn] Fullscreen app detected - recording paused (nothing will be saved)\r\n")
					state = paused
				} else if exitScreen {
					// Exiting fullscreen app → resume recording
					fmt.Fprintf(os.Stderr, "\r\n[tracelyn] Recording resumed - fullscreen app session was not recorded\r\n")
					state = waitingForEnter
					// Clear emulator to avoid capturing leftover screen data
					emulator = vt.NewEmulator(width, height)
					savedLines = []string{}
				}

				// Only feed to emulator when NOT paused
				currentState := state
				stateMutex.Unlock()

				if currentState != paused {
					emulator.Write(buf[:n])
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Goroutine 2: Read from stdin → detect Enter key + save on Enter only
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

				// Check if paused (in fullscreen app)
				stateMutex.Lock()
				currentState := state
				stateMutex.Unlock()

				if currentState == paused {
					continue
				}

				// Only save when Enter is pressed
				for i := 0; i < n; i++ {
					if buf[i] == '\r' || buf[i] == '\n' {
						// Wait for shell to process the command and update screen
						time.Sleep(50 * time.Millisecond)

						stateMutex.Lock()

						if state == waitingForEnter {
							// Save command line that was just entered
							saveErr := saveNewContent(emulator, sessionFile, &savedLines)
							if saveErr != nil {
								// Silently handle errors
							}
							state = waitingForOutput
						} else if state == waitingForOutput {
							// Save command output
							saveErr := saveNewContent(emulator, sessionFile, &savedLines)
							if saveErr != nil {
								// Silently handle errors
							}
							state = waitingForEnter
						}

						stateMutex.Unlock()
						break
					}
				}
			}
		}
	}()

	return nil
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
	// Create session file
	sessionFile, sessionPath, err := CreateSessionFile()
	if err != nil {
		return err
	}
	defer sessionFile.Close()

	// Create lock file with current PID
	pid := os.Getpid()
	if err := CreateLockFile(pid); err != nil {
		return err
	}
	defer RemoveLockFile()

	// Setup shell command
	cmd, err := SetupShellCommand()
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

	// Handle window resize
	resizeCh := HandleResize(ptmx)
	defer signal.Stop(resizeCh)

	// Handle shutdown signals (SIGTERM, SIGINT)
	sigChan := HandleShutdownSignals()
	defer signal.Stop(sigChan)

	// Setup I/O copy with recording
	if err := SetupIOCopy(ptmx, sessionFile); err != nil {
		return err
	}

	// Wait for shell to exit or shutdown signal
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-sigChan:
		// Received shutdown signal (SIGTERM or SIGINT)
		// Terminal will be restored by defer, lock removed, file closed
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

// StopRecording stops the current recording session by sending SIGTERM to the recording process
func StopRecording() error {
	// Read the lock file to get the recording process PID
	pid, err := ReadLockFile()
	if err != nil {
		return fmt.Errorf("no active recording session found")
	}

	// Check if process is actually running
	if !IsProcessRunning(pid) {
		// Clean up stale lock file
		RemoveLockFile()
		return fmt.Errorf("recording process is not running (stale lock file removed)")
	}

	// Send SIGTERM signal to gracefully stop the recording
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find recording process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop recording: %w", err)
	}

	return nil
}
