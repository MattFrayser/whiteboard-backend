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
func (sm *SessionManager) GetOrCreate(userID string) *UserSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[userID]
	if exists {
		session.LastSeen = time.Now()
		return session
	}

	// Create new session with persistent color
	session = &UserSession{
		UserID:      userID,
		LastSeen:    time.Now(),
		RateLimiter: rate.NewLimiter(30, 10), // 30 msg/sec, burst of 10
		Color:       getRandomHex(),
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
func (sm *SessionManager) GetLastSeen(userID string) (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, exists := sm.sessions[userID]; exists {
		return session.LastSeen, true
	}
	return time.Time{}, false
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
