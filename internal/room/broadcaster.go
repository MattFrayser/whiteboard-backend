package room

import (
	"log"
	"sync"

	"main/internal/user"

	"github.com/gorilla/websocket"
)

// RoomState: minimum interface for broadcasting
type RoomConnections interface {
	GetConnections() map[string]*user.User
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
	// snapshot of connections
	connections := rm.GetConnections()

	// list of users to broadcast to
	users := make([]*user.User, 0, len(connections))
	for _, u := range connections {
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
				log.Printf("Broadcast failed for user %s: %v", usr.ID, err)
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
