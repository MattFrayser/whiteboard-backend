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

// GenerateUUID generates a random UUID for user identification
func GenerateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// WriteMessage safely writes a message to the WebSocket connection with mutex protection
// gorilla/websocket does not allow concurrent writes
func (u *User) WriteMessage(messageType int, data []byte) error {
	u.WriteMutex.Lock()
	defer u.WriteMutex.Unlock()
	return u.Connection.WriteMessage(messageType, data)
}
