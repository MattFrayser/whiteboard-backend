package room

import (
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

	// Write outside lock to avoid blocking operations
	var failedUsers []*user.User
	for _, u := range users {
		if err := sender.WriteMessage(websocket.TextMessage, msg); err != nil {
			// Connection failed, mark for removal
			failedUsers = append(failedUsers, u)
		}
	}

	// Clean up failed connections
	for _, u := range failedUsers {
		rm.RemoveConnection(u.ID)
	}
}
