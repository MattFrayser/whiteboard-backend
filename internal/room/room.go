package room 

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"main/internal/user"

	"github.com/gorilla/websocket"
)

// Room represents a collaborative whiteboard room
type Room struct {
	Connections map[string]*user.User
	Objects     map[string]*DrawingObject
	LastActive  time.Time
	CreatedAt   time.Time
	mu          sync.RWMutex
}


// Join adds a user to the room and sends existing drawings
func (r *Room) Join(u *user.User, maxRoomSize int) error{
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check room size limit atomically with addition
	if len(r.Connections) >= maxRoomSize {
		return errors.New("room is full")
	}

	// add user to room
	r.Connections[u.ID] = u

	// Sync all objects
	objects := make([]map[string]interface{}, 0, len(r.Objects))
	for _, obj := range r.Objects {
		objects = append(objects, map[string]interface{}{
			"id":     obj.ID,
			"type":   obj.Type,
			"data":   obj.Data,
			"userId": obj.UserID,
			"zIndex": obj.ZIndex,
		})
	}

	syncMsg := map[string]interface{}{
		"type":    "sync",
		"objects": objects,
	}

	msgBytes, err := json.Marshal(syncMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal sync message: %w", err)
	}

	if err := u.Connection.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
		return fmt.Errorf("failed to send sync message: %w", err)
	}

	return nil
}

// Leave removes a user from the room
func (r *Room) Leave(u *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Connections, u.ID)

	r.LastActive = time.Now()
}

// Broadcast sends a message to all users except the sender
func (r *Room) Broadcast(msg []byte, sender *websocket.Conn) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.Connections {
		if u.Connection == sender {
			continue
		}

		// Best-effort send; read loop handles any cleanup 
		u.Connection.WriteMessage(websocket.TextMessage, msg)
	}
}

// AddObject adds a drawing object to the room
func (r *Room) AddObject(obj *DrawingObject) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Objects[obj.ID] = obj
	r.LastActive = time.Now()
}

// UpdateObject updates a drawing object in the room
func (r *Room) UpdateObject(id string, data map[string]interface{}) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if obj, exists := r.Objects[id]; exists {
		obj.Data = data
		r.LastActive = time.Now()
		return true
	}
	return false
}

// DeleteObject removes a drawing object from the room
func (r *Room) DeleteObject(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Objects, id)
	r.LastActive = time.Now()
}

// GetObjectCount returns the number of objects in the room
func (r *Room) GetObjectCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Objects)
}

// GetConnectionCount returns the number of connections in the room
func (r *Room) GetConnectionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Connections)
}
