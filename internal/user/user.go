package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// UserSession: persists across disconnects
type UserSession struct {
	UserID             string
	SessionToken       string
	TokenCreatedAt     time.Time     // For token rotation
	Fingerprint        string        // Browser fingerprint for token validation
	LastRoom           string
	LastSeen           time.Time
	LastCursorUpdate   time.Time
	ObjectRateLimiter  *rate.Limiter
	CursorRateLimiter  *rate.Limiter
	Color              string
	VerifiedRooms      map[string]time.Time // roomCode -> verification time (for password-protected rooms)
}

// User: connected user
type User struct {
	ID         string
	Session    *UserSession
	Connection *websocket.Conn
	WriteMutex sync.Mutex 
}

// GenerateUUID: generate random UUID for user identification
func GenerateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateSessionToken: generates session token
func GenerateSessionToken() string {
	bytes := make([]byte, 32) // 256-bit token
	if _, err := rand.Read(bytes); err != nil {
		panic("Crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(bytes)
}

// GenerateFingerprint: creates a fingerprint hash from User-Agent
func GenerateFingerprint(userAgent string) string {
	hash := sha256.Sum256([]byte(userAgent))
	return hex.EncodeToString(hash[:])
}

// WriteMessage: writes message to WebSocket connection 
// (gorilla/websocket does not allow concurrent writes)
func (u *User) WriteMessage(messageType int, data []byte) error {
	u.WriteMutex.Lock()
	defer u.WriteMutex.Unlock()

	return u.Connection.WriteMessage(messageType, data)
}
