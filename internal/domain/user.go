package domain

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lucasb-eyer/go-colorful"
	"golang.org/x/time/rate"
)

// UserSession persists across disconnects
type UserSession struct {
	UserID      string
	LastRoom    string
	LastSeen    time.Time
	RateLimiter *rate.Limiter
	Color       string
}

// User represents a connected user
type User struct {
	ID         string
	Session    *UserSession
	Connection *websocket.Conn
}

var (
	userSessions       = make(map[string]*UserSession)
	sessionsMutex      sync.RWMutex
	cursorColorCounter int
	cursorColorMutex   sync.Mutex
)

// GetOrCreateSession gets an existing session or creates a new one
func GetOrCreateSession(userID string) *UserSession {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	session, exists := userSessions[userID]
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
	userSessions[userID] = session
	return session
}

// GenerateUUID generates a random UUID for user identification
func GenerateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getRandomHex returns well-distributed hex colors using golden ratio
func getRandomHex() string {
	cursorColorMutex.Lock()
	defer cursorColorMutex.Unlock()

	const goldenRatio = 0.618033988749895
	hue := float64(cursorColorCounter) * goldenRatio
	hue = hue - float64(int(hue)) // Keep fractional part
	cursorColorCounter++

	color := colorful.Hsl(hue*360, 0.85, 0.55)
	return color.Hex()
}

// UpdateSessionLastSeen updates the last seen time for a user session
func UpdateSessionLastSeen(userID string, lastSeen time.Time) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	if session, exists := userSessions[userID]; exists {
		session.LastSeen = lastSeen
	}
}

// GetSessionLastSeen gets the last seen time for a user session
func GetSessionLastSeen(userID string) (time.Time, bool) {
	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()

	if session, exists := userSessions[userID]; exists {
		return session.LastSeen, true
	}
	return time.Time{}, false
}

// CleanupSessions removes expired user sessions
func CleanupSessions() {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	now := time.Now()
	for userID, session := range userSessions {
		// Remove sessions inactive for 1 hour
		if now.Sub(session.LastSeen) > 1*time.Hour {
			delete(userSessions, userID)
		}
	}
}
