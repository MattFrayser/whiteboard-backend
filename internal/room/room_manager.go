package room

import (
	"errors"
	"sync"
	"time"

	"main/internal/middleware"
	"main/internal/object"
	"main/internal/user"

)

// Manager manages all rooms in the application
type Manager struct {
	rooms map[string]*Room
	synchronizer Synchronizer
	mu    sync.RWMutex

}

// NewManager creates a new room manager
func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
}


func (rm *Manager) CreateRoom(roomCode string, maxRooms int) (*Room, error) {
	if roomCode == "" {
		return nil, errors.New("room code missing")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.rooms[roomCode] == nil {
		// Check global room limit before creating new room
		if len(rm.rooms) >= maxRooms {
			return nil, errors.New("server at maximum room capacity")
		}

		rm.rooms[roomCode] = &Room{
			Connections:    make(map[string]*user.User),
			Objects:        make(map[string]*object.Drawing),
			UserColors:     make(map[string]string),
			colorGenerator: user.NewColorGenerator(),
			LastActive:     time.Now(),
			CreatedAt:      time.Now(),
		}
	}

	room := rm.rooms[roomCode]

	return room, nil
}

// JoinRoom adds a user to a room, creating it if necessary
func (rm *Manager) JoinRoom(roomCode string, session *user.UserSession, u *user.User, rl *middleware.RateLimit) (*Room, error) {
	if roomCode == "" {
		return nil, errors.New("room code missing")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if user is rejoining their last room and it still exists
	if session.LastRoom == roomCode {
		if existingRoom, active := rm.GetRoom(roomCode); active {
			if err := existingRoom.Join(u, rl.MaxRoomSize); err != nil {
				return nil, err
			}
			if err := rm.synchronizer.SyncNewUser(existingRoom, u); err != nil {
				return nil, err
			}

			return existingRoom, nil
		}
	}

	// Either joining: different room, first time, room expired -> create/join new
	room, err := rm.CreateRoom(roomCode, rl.MaxRooms)
	if err != nil {
		return nil, err
	}

	if err := room.Join(u, rl.MaxRoomSize); err != nil {
		return nil, err
	}

	if err := rm.synchronizer.SyncNewUser(room, u); err != nil {
		return nil, err
	}

	return room, nil
}

// Cleanup removes expired rooms
func (rm *Manager) Cleanup() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()

	// Room removed if 1 hour empty or 24 hours old
	for code, room := range rm.rooms {
		room.mu.RLock()
		empty := len(room.Connections) == 0
		inactive := now.Sub(room.LastActive) > 1*time.Hour
		expired := now.Sub(room.CreatedAt) > 24*time.Hour
		room.mu.RUnlock()

		if (inactive && empty) || expired {
			delete(rm.rooms, code)
		}
	}
}

// GetRoom: checks if a room exists and returns it
func (rm *Manager) GetRoom(roomCode string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.rooms[roomCode]
	return room, exists
}

// GetRoomCount returns the total number of rooms
func (rm *Manager) RoomCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return len(rm.rooms)
}
