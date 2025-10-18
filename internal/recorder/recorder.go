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

// Debug log file (package-level variable)
var debugLogFile *os.File

// initDebugLog initializes the debug log file
func initDebugLog() error {
	dir, err := getTracelynDir()
	if err != nil {
		return err
	}

	logPath := filepath.Join(dir, "debug.log")
	debugLogFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create debug log: %w", err)
	}

	// Write timestamp header
	fmt.Fprintf(debugLogFile, "\n\n=== Recording session started at %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}

// logDebug writes to debug log file
func logDebug(format string, args ...interface{}) {
	if debugLogFile != nil {
		fmt.Fprintf(debugLogFile, format+"\n", args...)
		debugLogFile.Sync()
	}
}

// saveNewContent extracts and saves only new lines from the virtual terminal
// Compares line-by-line with previous screen state (allows repeated outputs)
// addSeparator adds a "|" separator after the last line when true
func saveNewContent(emulator *vt.Emulator, file *os.File, savedLines *[]string, addSeparator bool) error {
	height := emulator.Height()
	var currentLines []string

	// Extract all current non-empty lines
	for y := 0; y < height; y++ {
		line := extractLine(emulator, y)
		if len(line) > 0 {
			currentLines = append(currentLines, line)
		}
	}

	// Find the minimum common prefix between current and saved lines
	// This handles terminal scrolling and new content appearing
	minLen := len(*savedLines)
	if len(currentLines) < minLen {
		minLen = len(currentLines)
	}

	// Find where the content diverges
	divergeIdx := 0
	for i := 0; i < minLen; i++ {
		if (*savedLines)[i] == currentLines[i] {
			divergeIdx = i + 1
		} else {
			break
		}
	}

	// Debug logging
	newLinesCount := len(currentLines) - divergeIdx
	logDebug("saveNewContent called: addSeparator=%v, divergeIdx=%d, newLines=%d, totalLines=%d",
		addSeparator, divergeIdx, newLinesCount, len(currentLines))

	// Save all new lines after the divergence point
	for i := divergeIdx; i < len(currentLines); i++ {
		lineToWrite := currentLines[i]

		// Add separator to the FIRST new line if requested (command line only)
		// This is the line that contains the command the user just typed
		if addSeparator && i == divergeIdx {
			lineToWrite += " ||||"
			logDebug("  -> Adding separator to line %d: %q", i, lineToWrite)
		}

		if _, err := file.WriteString(lineToWrite + "\n"); err != nil {
			return fmt.Errorf("failed to write to session file: %w", err)
		}

		logDebug("  -> Saved line %d: %q", i, lineToWrite)
	}

	// Update saved lines to current screen state
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

	// Track saved lines (protected by mutex for goroutine safety)
	var stateMutex sync.Mutex
	var savedLines []string
	var paused bool

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
					paused = true
				} else if exitScreen {
					// Exiting fullscreen app → resume recording
					fmt.Fprintf(os.Stderr, "\r\n[tracelyn] Recording resumed - fullscreen app session was not recorded\r\n")
					paused = false
					// Clear emulator to avoid capturing leftover screen data
					emulator = vt.NewEmulator(width, height)
					savedLines = []string{}
				}

				// Only feed to emulator when NOT paused
				isPaused := paused
				stateMutex.Unlock()

				if !isPaused {
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
				isPaused := paused
				stateMutex.Unlock()

				if isPaused {
					continue
				}

				// Only save when Enter is pressed
				for i := 0; i < n; i++ {
					if buf[i] == '\r' || buf[i] == '\n' {
						// Wait for shell to process the command and update screen
						time.Sleep(50 * time.Millisecond)

						stateMutex.Lock()

						// Always save with separator (command lines get it, output doesn't matter)
						logDebug("Enter detected - saving content with separator")
						saveErr := saveNewContent(emulator, sessionFile, &savedLines, true)
						if saveErr != nil {
							logDebug("Error saving: %v", saveErr)
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
	// Initialize debug logging
	if err := initDebugLog(); err != nil {
		return err
	}
	defer func() {
		if debugLogFile != nil {
			debugLogFile.Close()
		}
	}()

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
