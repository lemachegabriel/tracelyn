package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

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
// Also accepts a callback to recreate the emulator with the new size
func HandleResize(ptmx *os.File, onResize func(width, height int)) chan os.Signal {
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

			// Get new terminal size
			width, height, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				continue // Keep old size if we can't read new one
			}

			// Call callback to recreate emulator with new size
			if onResize != nil {
				onResize(width, height)
			}
		}
	}()

	return resizeCh
}
