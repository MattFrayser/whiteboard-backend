package user

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// SessionManager manages user sessions
type SessionManager struct {
	sessions map[string]*UserSession
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*UserSession),
	}
}

// GetOrCreate gets an existing session or creates a new one
// Accepts a color parameter for new sessions
func (sm *SessionManager) GetOrCreate(userID string, color string) *UserSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[userID]
	if exists {
		session.LastSeen = time.Now()
		return session
	}

	// Create new session with provided color
	now := time.Now()
	session = &UserSession{
		UserID:           userID,
		LastSeen:         now,
		LastCursorUpdate: time.Time{},
		RateLimiter:      rate.NewLimiter(30, 10), // 30 msg/sec, burst of 10
		Color:            color,
	}
	sm.sessions[userID] = session
	return session
}

// UpdateLastSeen updates the last seen time for a user session
func (sm *SessionManager) UpdateLastSeen(userID string, lastSeen time.Time) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[userID]; exists {
		session.LastSeen = lastSeen
	}
}

// GetLastSeen gets the last seen time for a user session
func (sm *SessionManager) LastSeen(userID string) (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, exists := sm.sessions[userID]; exists {
		return session.LastSeen, true
	}
	return time.Time{}, false
}

// GetLastCursorUpdate gets the last cursor update time for a user session
func (sm *SessionManager) LastCursorUpdate(userID string) (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, exists := sm.sessions[userID]; exists {
		return session.LastCursorUpdate, true
	}
	return time.Time{}, false
}

// UpdateLastCursorUpdate updates the last cursor update time for a user session
func (sm *SessionManager) UpdateLastCursorUpdate(userID string, lastCursorUpdate time.Time) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[userID]; exists {
		session.LastCursorUpdate = lastCursorUpdate
	}
}

// Cleanup removes expired user sessions
func (sm *SessionManager) Cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for userID, session := range sm.sessions {
		// Remove sessions inactive for 1 hour
		if now.Sub(session.LastSeen) > 1*time.Hour {
			delete(sm.sessions, userID)
		}
	}
}
