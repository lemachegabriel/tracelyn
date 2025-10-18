package recorder

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/vt"
)

// trimRight removes trailing whitespace from string
func trimRight(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[:end]
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

// extractCurrentLines extracts all non-empty lines from the emulator
func extractCurrentLines(emulator *vt.Emulator) []string {
	height := emulator.Height()
	var currentLines []string

	for y := 0; y < height; y++ {
		line := extractLine(emulator, y)
		if len(line) > 0 {
			currentLines = append(currentLines, line)
		}
	}

	return currentLines
}

// findSequenceMatch finds the best matching sequence between saved and current lines
// Returns the index in currentLines where new content starts
func findSequenceMatch(savedLines, currentLines []string) int {
	if len(savedLines) == 0 || len(currentLines) == 0 {
		return 0
	}

	// Try to find overlap by searching for sequences of lines
	// Start with longer sequences (more reliable) and work down to 3 lines minimum
	maxSequenceLen := 10
	minSequenceLen := 3
	if len(savedLines) < maxSequenceLen {
		maxSequenceLen = len(savedLines)
	}

	bestMatch := 0

	// Try progressively smaller sequence lengths
	for seqLen := maxSequenceLen; seqLen >= minSequenceLen; seqLen-- {
		// Search for sequences from savedLines (start from end, go backwards)
		for savedIdx := len(savedLines) - seqLen; savedIdx >= 0; savedIdx-- {
			savedSequence := savedLines[savedIdx : savedIdx+seqLen]

			// Search for this sequence anywhere in currentLines
			for currIdx := 0; currIdx <= len(currentLines)-seqLen; currIdx++ {
				// Check if sequence matches at position currIdx
				matches := true
				for j := 0; j < seqLen; j++ {
					if currentLines[currIdx+j] != savedSequence[j] {
						matches = false
						break
					}
				}

				if matches {
					// Found a sequence match!
					// Position after this sequence in currentLines
					matchEnd := currIdx + seqLen

					// Prefer matches that are closer to the end of currentLines
					// (more recent content is more likely to be the right anchor)
					if matchEnd > bestMatch {
						bestMatch = matchEnd
					}

					// If we found a long sequence near the end, we can stop
					if seqLen >= 5 && matchEnd >= len(currentLines)-5 {
						return bestMatch
					}
				}
			}
		}
	}

	if bestMatch > 0 {
		return bestMatch
	}

	// Fallback: check for simple overlap from the start
	minLen := len(savedLines)
	if len(currentLines) < minLen {
		minLen = len(currentLines)
	}

	divergeIdx := 0
	for i := 0; i < minLen; i++ {
		if savedLines[i] == currentLines[i] {
			divergeIdx = i + 1
		} else {
			break
		}
	}

	return divergeIdx
}

// findCommandLineIndex finds the command line in current lines (searches from bottom up)
func findCommandLineIndex(currentLines []string, commandLine string) int {
	if commandLine == "" {
		return -1
	}

	for i := len(currentLines) - 1; i >= 0; i-- {
		if currentLines[i] == commandLine {
			return i
		}
	}

	return -1
}

// isCommandLineSaved checks if command line was already saved with separator
func isCommandLineSaved(savedLines []string, commandLine string) bool {
	if commandLine == "" {
		return false
	}

	cmdLineWithSep := commandLine + " ||||"
	for _, saved := range savedLines {
		if saved == cmdLineWithSep {
			return true
		}
	}

	return false
}

// saveNewContent extracts and saves only new lines from the virtual terminal
// Compares line-by-line with previous screen state (allows repeated outputs)
// commandLine is the line content that should receive the separator (captured when Enter was pressed)
func saveNewContent(emulator *vt.Emulator, file *os.File, savedLines *[]string, commandLine string) error {
	currentLines := extractCurrentLines(emulator)

	// Find where to start saving from currentLines
	divergeIdx := findSequenceMatch(*savedLines, currentLines)

	// Check if command line needs to be saved (regardless of divergeIdx)
	cmdLineIdx := findCommandLineIndex(currentLines, commandLine)
	cmdLineNeedsSave := cmdLineIdx >= 0 && !isCommandLineSaved(*savedLines, commandLine)

	// Save all new lines after the divergence point
	savedCmdLine := false
	for i := divergeIdx; i < len(currentLines); i++ {
		lineToWrite := currentLines[i]

		// Add separator if this line matches the command line (the line where Enter was pressed)
		if commandLine != "" && lineToWrite == commandLine {
			lineToWrite += " ||||"
			savedCmdLine = true
		}

		if _, err := file.WriteString(lineToWrite + "\n"); err != nil {
			return fmt.Errorf("failed to write to session file: %w", err)
		}
	}

	// If command line needs saving but wasn't saved in the loop above
	// (because it's before divergeIdx), save it AND all output after it until divergeIdx
	if cmdLineNeedsSave && !savedCmdLine {
		cmdLineWithSep := commandLine + " ||||"

		// Save command line with separator
		if _, err := file.WriteString(cmdLineWithSep + "\n"); err != nil {
			return fmt.Errorf("failed to write to session file: %w", err)
		}

		// Save all lines between command line and divergeIdx (the output of the command)
		for i := cmdLineIdx + 1; i < divergeIdx; i++ {
			if _, err := file.WriteString(currentLines[i] + "\n"); err != nil {
				return fmt.Errorf("failed to write to session file: %w", err)
			}
		}

		// Update savedLines to current state
		*savedLines = currentLines

		// Flush to disk
		if err := file.Sync(); err != nil {
			return fmt.Errorf("failed to sync session file: %w", err)
		}
		return nil
	}

	// Update saved lines to current screen state
	*savedLines = currentLines

	// Flush to disk
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync session file: %w", err)
	}

	return nil
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
