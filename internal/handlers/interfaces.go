package handlers

import (
	"time"

	"main/internal/object"
	"main/internal/room"

	"github.com/gorilla/websocket"
)

// Broadcaster defines the broadcast operation for sending messages to room users
type Broadcaster interface {
	Broadcast(rm room.RoomConnections, msg []byte, sender *websocket.Conn)
}

// SessionProvider defines operations for managing user sessions
type SessionProvider interface {
	LastCursorUpdate(userID string) (time.Time, bool)
	UpdateLastCursorUpdate(userID string, t time.Time)
}

// RoomObjects defines the interface for rooms that object handlers need
type RoomObjects interface {
	room.RoomConnections // Embed for broadcasting support

	AddObject(obj *object.Drawing)
	UpdateObject(id string, data map[string]interface{}) bool
	GetObject(id string) *object.Drawing
	DeleteObject(id string)
	ObjectCount() int
}
