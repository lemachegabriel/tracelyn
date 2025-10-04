package recorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSessionFile(t *testing.T) {
	// Clean up any existing sessions directory
	defer os.RemoveAll("sessions")

	file, path, err := CreateSessionFile()
	if err != nil {
		t.Fatalf("createSessionFile() failed: %v", err)
	}
	defer file.Close()

	t.Logf("Created session file: %s", path)

	// Check file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("session file was not created at %s", path)
	}

	// Check filename format
	filename := filepath.Base(path)
	if !strings.HasPrefix(filename, "session_") || !strings.HasSuffix(filename, ".txt") {
		t.Errorf("unexpected filename format: %s", filename)
	}

	// Check sessions directory exists
	if _, err := os.Stat("sessions"); os.IsNotExist(err) {
		t.Error("sessions directory was not created")
	}
}
