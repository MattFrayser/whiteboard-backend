package domain

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Room represents a collaborative whiteboard room
type Room struct {
	Connections []*User
	Objects     map[string]*DrawingObject
	LastActive  time.Time
	CreatedAt   time.Time
	mu          sync.RWMutex
}

var (
	rooms      = make(map[string]*Room)
	roomsMutex sync.RWMutex
)

// Join adds a user to the room and sends existing drawings
func (r *Room) Join(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Connections = append(r.Connections, u)

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

	msgBytes, _ := json.Marshal(syncMsg)
	u.Connection.WriteMessage(websocket.TextMessage, msgBytes)
}

// Leave removes a user from the room
func (r *Room) Leave(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, v := range r.Connections {
		if v == u {
			r.Connections = append(r.Connections[:i], r.Connections[i+1:]...)
			break
		}
	}

	r.LastActive = time.Now()
}

// Broadcast sends a message to all users except the sender
func (r *Room) Broadcast(msg []byte, sender *websocket.Conn) {
	r.mu.RLock()
	connections := make([]*User, len(r.Connections))
	copy(connections, r.Connections)
	r.mu.RUnlock()

	var failed []*User
	for _, u := range connections {
		if u.Connection == sender {
			continue
		}

		if err := u.Connection.WriteMessage(websocket.TextMessage, msg); err != nil {
			failed = append(failed, u)
		}
	}

	// Remove failed connections
	if len(failed) > 0 {
		r.mu.Lock()
		for _, failedUser := range failed {
			for i, u := range r.Connections {
				if u == failedUser {
					r.Connections = append(r.Connections[:i], r.Connections[i+1:]...)
					break
				}
			}
		}
		r.mu.Unlock()
	}
}

// UpdateLastActive updates the last active time
func (r *Room) UpdateLastActive() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.LastActive = time.Now()
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

// GetRoomIfActive checks if a room exists and returns it
func GetRoomIfActive(roomCode string) (*Room, bool) {
	roomsMutex.RLock()
	defer roomsMutex.RUnlock()

	room, exists := rooms[roomCode]
	return room, exists
}

// JoinRoom adds a user to a room, creating it if necessary
func JoinRoom(roomCode string, user *User, maxRooms, maxRoomSize int) (*Room, error) {
	if roomCode == "" {
		return nil, errors.New("room code missing")
	}

	roomsMutex.Lock()
	defer roomsMutex.Unlock()

	if rooms[roomCode] == nil {
		// Check global room limit before creating new room
		if len(rooms) >= maxRooms {
			return nil, errors.New("server at maximum room capacity")
		}

		rooms[roomCode] = &Room{
			Connections: []*User{},
			Objects:     make(map[string]*DrawingObject),
			LastActive:  time.Now(),
			CreatedAt:   time.Now(),
		}
	}

	room := rooms[roomCode]

	// Check room size limit before joining
	if len(room.Connections) >= maxRoomSize {
		return nil, errors.New("room is full")
	}

	room.Join(user)
	return room, nil
}

// GetRoomCount returns the total number of rooms
func GetRoomCount() int {
	roomsMutex.RLock()
	defer roomsMutex.RUnlock()
	return len(rooms)
}

// CleanupRooms removes expired rooms
func CleanupRooms() {
	roomsMutex.Lock()
	defer roomsMutex.Unlock()

	now := time.Now()

	// Room removed if 1 hour empty or 24 hours old
	for code, room := range rooms {
		room.mu.RLock()
		empty := len(room.Connections) == 0
		inactive := now.Sub(room.LastActive) > 1*time.Hour
		expired := now.Sub(room.CreatedAt) > 24*time.Hour
		room.mu.RUnlock()

		if (inactive && empty) || expired {
			delete(rooms, code)
		}
	}
}
