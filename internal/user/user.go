package user 

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
	cursorColorCounter int
	cursorColorMutex   sync.Mutex
)

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
