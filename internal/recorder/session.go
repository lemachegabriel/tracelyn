package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Session represents a single recording session
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	FilePath  string    `json:"file_path"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionRegistry manages all active recording sessions
type SessionRegistry struct {
	Sessions []Session  `json:"sessions"`
	mu       sync.Mutex // For concurrent access
}

const registryFile = ".tracelyn.sessions"

// LoadSessions loads the session registry from file, creates if not exists
func LoadSessions() (*SessionRegistry, error) {
	registry := &SessionRegistry{
		Sessions: []Session{},
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
		CreatedAt: time.Now(),
	}

	r.Sessions = append(r.Sessions, *session)
	return session
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

// RemoveSession removes a session from the registry
func (r *SessionRegistry) RemoveSession(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, session := range r.Sessions {
		if session.ID == id {
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
