package room

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"main/internal/middleware"
	"main/internal/object"
	"main/internal/user"
)

// Manager manages all rooms in the application
type Manager struct {
	rooms map[string]*Room
	synchronizer *Synchronizer
	mu    sync.RWMutex

}

// NewManager creates a new room manager
func NewManager() *Manager {
	return &Manager{
		rooms:        make(map[string]*Room),
		synchronizer: NewSynchronizer(),
	}
}

// compile regex once 
var roomCodeRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)


// RoomSettings defines configuration for creating a room
type RoomSettings struct {
	Password    string          // Hashed password (already hashed by caller)
	Permissions RoomPermissions // Access control settings
	CreatedBy   string          // Creator's userId
}

// CreateRoom: helper to join
// no need to check roomCode or lock, this should only be called from join
func (rm *Manager) createRoom(roomCode string, maxRooms int) (*Room, error) {
	return rm.createRoomWithSettings(roomCode, maxRooms, nil)
}

// CreateRoomWithSettings creates a new room with specific settings
// Returns existing room if it already exists (does not overwrite)
func (rm *Manager) createRoomWithSettings(roomCode string, maxRooms int, settings *RoomSettings) (*Room, error) {
	if rm.rooms[roomCode] == nil {
		// Check global room limit before creating new room
		if len(rm.rooms) >= maxRooms {
			return nil, errors.New("server at maximum room capacity")
		}

		room := &Room{
			Connections:    make(map[string]*user.User),
			Objects:        make(map[string]*object.Drawing),
			UserColors:     make(map[string]string),
			colorGenerator: user.NewColorGenerator(),
			LastActive:     time.Now(),
			CreatedAt:      time.Now(),
		}

		// Apply settings if provided
		if settings != nil {
			room.Password = settings.Password
			room.Permissions = settings.Permissions
			room.CreatedBy = settings.CreatedBy
		}

		rm.rooms[roomCode] = room
	}

	return rm.rooms[roomCode], nil
}

// CreateRoomWithSettingsPublic is the public API for creating rooms with settings
// Validates room code and acquires lock before creating
func (rm *Manager) CreateRoomWithSettingsPublic(roomCode string, settings *RoomSettings, rl *middleware.RateLimit) (*Room, error) {
	if err := rm.validateRoomCode(roomCode); err != nil {
		return nil, errors.New("invalid room code")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	return rm.createRoomWithSettings(roomCode, rl.MaxRooms, settings)
}

// JoinRoom adds a user to a room, creating it if necessary
func (rm *Manager) JoinRoom(roomCode string, session *user.UserSession, u *user.User, rl *middleware.RateLimit) (*Room, error) {

	if err := rm.validateRoomCode(roomCode); err != nil {
		return nil, errors.New("invalid room code")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if user is rejoining their last room and it still exists
	if session.LastRoom == roomCode {
		if existingRoom, active := rm.rooms[roomCode]; active {
			if err := existingRoom.Join(u, rl.MaxRoomSize); err != nil {
				return nil, err
			}
			return existingRoom, nil
		}
	}

	// Either joining: different room, first time, room expired -> create/join new
	// Set the first joiner as the creator for auto-created rooms
	room, err := rm.createRoomWithSettings(roomCode, rl.MaxRooms, &RoomSettings{
		CreatedBy: u.ID,
	})
	if err != nil {
		return nil, err
	}

	if err := room.Join(u, rl.MaxRoomSize); err != nil {
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

func (rm *Manager) validateRoomCode(code string) error {
    if len(code) != 6 {
        return fmt.Errorf("invalid room code length")
    }
    if !roomCodeRegex.MatchString(code) {
        return fmt.Errorf("invalid room code format")
    }
    return nil
}
