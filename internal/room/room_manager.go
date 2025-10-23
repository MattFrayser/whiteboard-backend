package room

import (
	"errors"
	"sync"
	"time"

	"main/internal/user"
)

var (
	rooms      = make(map[string]*Room)
	roomsMutex sync.RWMutex
)

// JoinRoom adds a user to a room, creating it if necessary
func JoinRoom(roomCode string, u *user.User, maxRooms, maxRoomSize int) (*Room, error) {
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
			Connections: make(map[string]*user.User),
			Objects:     make(map[string]*DrawingObject),
			LastActive:  time.Now(),
			CreatedAt:   time.Now(),
		}
	}

	// Attempt join
	room := rooms[roomCode]

	if err := room.Join(u, maxRoomSize); err != nil {
		return nil, err
	}

	return room, nil
}


// CleanupRooms removes expired rooms
func Cleanup() {
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

// GetRoomIfActive checks if a room exists and returns it
func GetRoomIfActive(roomCode string) (*Room, bool) {
	roomsMutex.RLock()
	defer roomsMutex.RUnlock()

	room, exists := rooms[roomCode]
	return room, exists
}

// GetRoomCount returns the total number of rooms
func GetRoomCount() int {
	roomsMutex.RLock()
	defer roomsMutex.RUnlock()
	return len(rooms)
}
