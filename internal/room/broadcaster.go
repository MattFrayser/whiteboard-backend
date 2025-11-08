package room

import (
	"sync"

	"main/internal/logger"
	"main/internal/user"

	"github.com/gorilla/websocket"
)

// RoomConnections: minimum interface for broadcasting
type RoomConnections interface {
	GetConnections() []*user.User
	RemoveConnection(userID string)
	GetUserColor(userID string) string
}

// Broadcaster: handles broadcasting messages to room users
type Broadcaster struct{}

// NewBroadcaster: creates a new broadcaster
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{}
}

// Broadcast: sends a message to all users in a room (except the sender)
func (b *Broadcaster) Broadcast(rm RoomConnections, msg []byte, sender *websocket.Conn) {
	// Get snapshot of connections as slice (already filtered copy from Room)
	allUsers := rm.GetConnections()

	// Filter out sender
	users := make([]*user.User, 0, len(allUsers))
	for _, u := range allUsers {
		if u.Connection != sender {
			users = append(users, u)
		}
	}

	// Concurrent write to all users
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failedUsers []*user.User

	for _, u := range users {
		wg.Add(1)
		go func(usr *user.User) {
			defer wg.Done()

			if err := usr.WriteMessage(websocket.TextMessage, msg); err != nil {
				logger.Error("Broadcast failed for user").
					Str("user_id", usr.ID).
					Err(err).
					Msg("")
				mu.Lock()
				failedUsers = append(failedUsers, usr)
				mu.Unlock()
			}
		}(u)
	}

	wg.Wait()

	// Clean up failed connections
	for _, u := range failedUsers {
		// remove from room 
		rm.RemoveConnection(u.ID)
		// Close WebSocket connection
		u.Connection.Close()
	}
}
