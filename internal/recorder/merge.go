package recorder

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"time"
)

// SessionLine represents a line from a session file with metadata
type SessionLine struct {
	Timestamp time.Time
	SessionID string
	Content   string
}

// MergeSessions merges multiple session files into one, sorted chronologically
func MergeSessions(sessionIDs []string, outputPath string) error {
	registry, err := LoadSessions()
	if err != nil {
		return err
	}

	var allLines []SessionLine

	// Read all session files
	for _, id := range sessionIDs {
		session, err := registry.GetSession(id)
		if err != nil {
			return fmt.Errorf("session %s not found", id)
		}

		lines, err := readSessionFile(session.FilePath, id, session.CreatedAt)
		if err != nil {
			return err
		}
		allLines = append(allLines, lines...)
	}

	// Sort by timestamp
	sort.Slice(allLines, func(i, j int) bool {
		return allLines[i].Timestamp.Before(allLines[j].Timestamp)
	})

	// Write merged file
	return writeMergedFile(allLines, outputPath)
}

// readSessionFile reads a session file and returns lines with estimated timestamps
func readSessionFile(filePath, sessionID string, baseTime time.Time) ([]SessionLine, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file %s: %w", filePath, err)
	}
	defer file.Close()

	var lines []SessionLine
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		content := scanner.Text()

		// Estimate timestamp based on line order
		// Assume each line takes ~1 second on average (simple heuristic for MVP)
		estimatedTime := baseTime.Add(time.Duration(lineNumber) * time.Second)

		lines = append(lines, SessionLine{
			Timestamp: estimatedTime,
			SessionID: sessionID,
			Content:   content,
		})

		lineNumber++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading session file %s: %w", filePath, err)
	}

	return lines, nil
}

// writeMergedFile writes merged lines to output file with metadata
func writeMergedFile(lines []SessionLine, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	for _, line := range lines {
		// Write with metadata: [HH:MM:SS][Session ID] content
		_, err := fmt.Fprintf(file, "[%s][Session %s] %s\n",
			line.Timestamp.Format("15:04:05"),
			line.SessionID,
			line.Content)
		if err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
	}

	return nil
}
