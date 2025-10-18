package recorder

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
	"golang.org/x/term"
)

// SetupIOCopy sets up bidirectional I/O copying between the terminal and PTY
// with Enter-based saving: only saves content when Enter is pressed
// Automatically pauses recording when fullscreen apps (vim, less) are detected
// Returns a function that should be called when terminal is resized
func SetupIOCopy(ptmx *os.File, sessionFile *os.File) (onResize func(int, int), err error) {
	// Get current terminal size for virtual terminal
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to standard 80x24 if we can't get size
		width, height = 80, 24
	}

	// Create virtual terminal emulator with MUCH larger buffer to capture all output
	// Even if output is longer than terminal height, we keep it in the buffer
	var emulator *vt.Emulator
	bufferHeight := height * 100 // 100x terminal height (e.g., 60 lines × 100 = 6000 lines buffer)
	emulator = vt.NewEmulator(width, bufferHeight)

	// Track saved lines (protected by mutex for goroutine safety)
	var stateMutex sync.Mutex
	var savedLines []string
	var paused bool

	// Create resize callback that recreates emulator with new size
	onResize = func(newWidth, newHeight int) {
		stateMutex.Lock()
		defer stateMutex.Unlock()

		// Recreate emulator with new size (100x buffer height)
		newBufferHeight := newHeight * 100
		emulator = vt.NewEmulator(newWidth, newBufferHeight)
		width, height = newWidth, newHeight

		// Clear saved lines to avoid mismatches with new screen size
		savedLines = []string{}
	}

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
						// Capture the command line BEFORE any output appears
						// Get cursor position to know which line contains the command
						stateMutex.Lock()
						cursorPos := emulator.CursorPosition()
						commandLine := extractLine(emulator, cursorPos.Y)

						// Check if this is the same as the last saved line (empty Enter on prompt)
						// If so, skip saving to avoid duplicates
						isEmptyEnter := len(savedLines) > 0 && commandLine == savedLines[len(savedLines)-1]
						if isEmptyEnter {
							stateMutex.Unlock()
							break
						}
						stateMutex.Unlock()

						// Wait for shell to process the command and update screen
						time.Sleep(50 * time.Millisecond)

						stateMutex.Lock()
						saveNewContent(emulator, sessionFile, &savedLines, commandLine)

						stateMutex.Unlock()
						break
					}
				}
			}
		}
	}()

	return onResize, nil
}
