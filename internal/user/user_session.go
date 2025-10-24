package user

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type SessionManager struct {
	sessions      map[string]*UserSession // userID -> session
	tokenToUserID map[string]string       // token -> userID
	mu            sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:      make(map[string]*UserSession),
		tokenToUserID: make(map[string]string),
	}
}

// GetOrCreate: gets an existing session or creates a new one
func (sm *SessionManager) GetOrCreate(userID string, color string) *UserSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[userID]
	if exists {
		session.LastSeen = time.Now()
		return session
	}

	// Create new session with generated token
	now := time.Now()
	token := GenerateSessionToken()
	session = &UserSession{
		UserID:            userID,
		SessionToken:      token,
		LastSeen:          now,
		LastCursorUpdate:  time.Time{},
		ObjectRateLimiter: rate.NewLimiter(30, 10), // 30 msg/sec, burst of 10 for objects
		CursorRateLimiter: rate.NewLimiter(60, 20), // 60 msg/sec, burst of 20 for cursor
	}
	sm.sessions[userID] = session
	sm.tokenToUserID[token] = userID
	return session
}

// ValidateToken: validate session token and returns the associated userID
func (sm *SessionManager) ValidateToken(token string) (string, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	userID, exists := sm.tokenToUserID[token]
	if !exists {
		return "", false
	}

	// Verify the session still exists
	session, sessionExists := sm.sessions[userID]
	if !sessionExists {
		return "", false
	}

	// Update last seen
	session.LastSeen = time.Now()
	return userID, true
}

// GetSessionByToken: retrieve session by token
func (sm *SessionManager) GetSessionByToken(token string) (*UserSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	userID, exists := sm.tokenToUserID[token]
	if !exists {
		return nil, false
	}

	session, sessionExists := sm.sessions[userID]
	return session, sessionExists
}

// UpdateTokenMapping: updates the token-to-userID mapping
// Used when overriding a session's token
func (sm *SessionManager) UpdateTokenMapping(token string, userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.tokenToUserID[token] = userID
}

// UpdateLastSeen: update last seen time for a user session
func (sm *SessionManager) UpdateLastSeen(userID string, lastSeen time.Time) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[userID]; exists {
		session.LastSeen = lastSeen
	}
}

// LastSeen: gets the last seen time for a user session
func (sm *SessionManager) LastSeen(userID string) (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, exists := sm.sessions[userID]; exists {
		return session.LastSeen, true
	}
	return time.Time{}, false
}

// LastCursor: gets the last cursor update time for a user session
func (sm *SessionManager) LastCursor(userID string) (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if session, exists := sm.sessions[userID]; exists {
		return session.LastCursorUpdate, true
	}
	return time.Time{}, false
}

// UpdateLastCursor: updates the last cursor update time for a user session
func (sm *SessionManager) UpdateLastCursor(userID string, lastCursorUpdate time.Time) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[userID]; exists {
		session.LastCursorUpdate = lastCursorUpdate
	}
}

// Remove:  removes a user session (called on disconnect)
func (sm *SessionManager) Remove(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Remove token mapping if session exists
	if session, exists := sm.sessions[userID]; exists {
		delete(sm.tokenToUserID, session.SessionToken)
	}

	delete(sm.sessions, userID)
}

// Cleanup: removes expired user sessions
func (sm *SessionManager) Cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for userID, session := range sm.sessions {
		// Remove sessions inactive for 1 hour
		if now.Sub(session.LastSeen) > 1*time.Hour {
			delete(sm.tokenToUserID, session.SessionToken)
			delete(sm.sessions, userID)
		}
	}
}
