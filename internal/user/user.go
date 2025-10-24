package user 

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// UserSession: persists across disconnects
type UserSession struct {
	UserID           string
	SessionToken     string 
	LastRoom         string
	LastSeen         time.Time
	LastCursorUpdate time.Time
	RateLimiter      *rate.Limiter
	Color            string
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
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// WriteMessage: writes message to WebSocket connection 
// (gorilla/websocket does not allow concurrent writes)
func (u *User) WriteMessage(messageType int, data []byte) error {
	u.WriteMutex.Lock()
	defer u.WriteMutex.Unlock()

	return u.Connection.WriteMessage(messageType, data)
}
