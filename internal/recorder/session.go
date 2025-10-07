package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Session represents a single recording session
type Session struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	FilePath    string     `json:"file_path"`
	PID         int        `json:"pid"`
	Status      string     `json:"status"` // "active" or "completed"
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// SessionRegistry manages all active recording sessions
type SessionRegistry struct {
	Sessions []Session  `json:"sessions"`
	mu       sync.Mutex // For concurrent access
}

// getTracelynDir returns the tracelyn directory path (~/.tracelyn)
func getTracelynDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".tracelyn"), nil
}

// getRegistryFilePath returns the full path to the registry file
func getRegistryFilePath() (string, error) {
	dir, err := getTracelynDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions.json"), nil
}

// getSessionsDir returns the sessions directory path (~/.tracelyn/sessions)
func getSessionsDir() (string, error) {
	dir, err := getTracelynDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// ensureTracelynDirs creates the tracelyn directories if they don't exist
func ensureTracelynDirs() error {
	dir, err := getTracelynDir()
	if err != nil {
		return err
	}

	// Create ~/.tracelyn directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create tracelyn directory: %w", err)
	}

	// Create ~/.tracelyn/sessions directory
	sessionsDir, err := getSessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	return nil
}

// LoadSessions loads the session registry from file, creates if not exists
func LoadSessions() (*SessionRegistry, error) {
	// Ensure directories exist
	if err := ensureTracelynDirs(); err != nil {
		return nil, err
	}

	registry := &SessionRegistry{
		Sessions: []Session{},
	}

	registryFile, err := getRegistryFilePath()
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(registryFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry file: %w", err)
	}
	defer file.Close()

	// Acquire shared lock for reading
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("failed to lock registry file: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	// Read file info to check if it's empty
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat registry file: %w", err)
	}

	// If file is empty, return empty registry
	if fileInfo.Size() == 0 {
		return registry, nil
	}

	// Decode JSON from file
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(registry); err != nil {
		return nil, fmt.Errorf("failed to decode registry: %w", err)
	}

	return registry, nil
}

// Save saves the session registry to file with file locking
func (r *SessionRegistry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	registryFile, err := getRegistryFilePath()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(registryFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open registry file: %w", err)
	}
	defer file.Close()

	// Acquire exclusive lock for writing
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock registry file: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	// Encode JSON to file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(r); err != nil {
		return fmt.Errorf("failed to encode registry: %w", err)
	}

	return nil
}

// CreateSession creates a new session with auto-incremented ID
func (r *SessionRegistry) CreateSession(name, filePath string, pid int) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find next available ID
	maxID := 0
	for _, s := range r.Sessions {
		id, err := strconv.Atoi(s.ID)
		if err == nil && id > maxID {
			maxID = id
		}
	}

	session := &Session{
		ID:        strconv.Itoa(maxID + 1),
		Name:      name,
		FilePath:  filePath,
		PID:       pid,
		Status:    "active",
		CreatedAt: time.Now(),
	}

	r.Sessions = append(r.Sessions, *session)
	return session
}

// CompleteSession marks a session as completed
func (r *SessionRegistry) CompleteSession(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Sessions {
		if r.Sessions[i].ID == id {
			r.Sessions[i].Status = "completed"
			now := time.Now()
			r.Sessions[i].CompletedAt = &now
			return nil
		}
	}

	return fmt.Errorf("session not found: %s", id)
}

// GetSession gets a session by ID
func (r *SessionRegistry) GetSession(id string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Sessions {
		if r.Sessions[i].ID == id {
			return &r.Sessions[i], nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", id)
}

// RemoveSession removes a session from the registry and deletes the session file
func (r *SessionRegistry) RemoveSession(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, session := range r.Sessions {
		if session.ID == id {
			// Only allow removing completed sessions
			if session.Status == "active" {
				return fmt.Errorf("cannot remove active session: %s (use 'tracelyn stop %s' first)", id, id)
			}

			// Delete the session file
			if err := os.Remove(session.FilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to delete session file: %w", err)
			}

			// Remove from registry
			r.Sessions = append(r.Sessions[:i], r.Sessions[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("session not found: %s", id)
}

// ListActiveSessions returns only sessions with running PIDs
func (r *SessionRegistry) ListActiveSessions() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	var active []Session
	for _, session := range r.Sessions {
		if IsProcessRunning(session.PID) {
			active = append(active, session)
		}
	}

	return active
}

// ListAllSessions returns all sessions with their active status
func (r *SessionRegistry) ListAllSessions() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.Sessions
}

// IsSessionActive checks if a session's process is still running
func (r *SessionRegistry) IsSessionActive(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, session := range r.Sessions {
		if session.ID == sessionID {
			return IsProcessRunning(session.PID)
		}
	}

	return false
}

// CleanupStaleSessions removes sessions with dead PIDs
func (r *SessionRegistry) CleanupStaleSessions() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var activeSessions []Session
	for _, session := range r.Sessions {
		if IsProcessRunning(session.PID) {
			activeSessions = append(activeSessions, session)
		}
	}

	r.Sessions = activeSessions
	return nil
}

// GetSessionByPID finds a session by PID
func (r *SessionRegistry) GetSessionByPID(pid int) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Sessions {
		if r.Sessions[i].PID == pid {
			return &r.Sessions[i], nil
		}
	}

	return nil, fmt.Errorf("no session found for PID %d", pid)
}

