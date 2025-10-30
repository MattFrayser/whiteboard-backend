package room

import (
	"errors"
	"sync"
	"time"

	"main/internal/user"
	"main/internal/object"
)

// RoomPermissions defines access control settings for a room
type RoomPermissions struct {
	ViewOnly    bool // All users except creator are view-only
	OnlyEditOwn bool // Users can only edit objects they created
}

// Room represents a collaborative whiteboard room
type Room struct {
	Connections    map[string]*user.User
	Objects        map[string]*object.Drawing
	UserColors     map[string]string // userID → color (room-specific)
	colorGenerator *user.ColorGenerator
	LastActive     time.Time
	CreatedAt      time.Time

	// Security and permissions
	Password    string          // Hashed password (empty if no password)
	Permissions RoomPermissions // Access control settings
	CreatedBy   string          // Creator's userId (has full permissions)

	mu             sync.RWMutex
}


// Join: adds user to room and assigns a unique color
func (r *Room) Join(u *user.User, maxRoomSize int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Connections) >= maxRoomSize {
		return errors.New("room is full")
	}

	r.Connections[u.ID] = u

	// Assign color if user doesn't have one in this room yet
	if _, hasColor := r.UserColors[u.ID]; !hasColor {
		r.UserColors[u.ID] = r.colorGenerator.NextColor()
	}

	return nil
}

// Leave: remove  user from room
func (r *Room) Leave(u *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Connections, u.ID)

	r.LastActive = time.Now()
}


// AddObject: adds drawing to room
func (r *Room) AddObject(obj *object.Drawing) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Objects[obj.ID] = obj
	r.LastActive = time.Now()
}

// UpdateObject: updates drawing in room
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

// DeleteObject: removes drawing from room
func (r *Room) DeleteObject(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Objects, id)
	r.LastActive = time.Now()
}

// GetObject: retrieves drawing from room (by ID)
func (r *Room) GetObject(id string) *object.Drawing {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Objects[id]
}

// GetObjectCount: returns number of objects in room
func (r *Room) ObjectCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.Objects)
}

// GetConnectionCount: returns number of connections in room
func (r *Room) ConnectionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.Connections)
}

// GetConnections: returns snapshot of current connections as a slice (for broadcasting)
func (r *Room) GetConnections() []*user.User {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return slice directly - broadcaster needs a slice anyway
	// This eliminates redundant map->slice conversion
	snapshot := make([]*user.User, 0, len(r.Connections))
	for _, v := range r.Connections {
		snapshot = append(snapshot, v)
	}
	return snapshot
}

// RemoveConnection: removes user connection from room (cleanup after failed broadcast)
func (r *Room) RemoveConnection(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Connections, userID)
}

// GetUserColor: returns the user's color in this room
func (r *Room) GetUserColor(userID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.UserColors[userID]
}

// IsCreator: checks if the given userId is the room creator
func (r *Room) IsCreator(userID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.CreatedBy == userID
}

// HasPassword: checks if the room is password-protected
func (r *Room) HasPassword() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Password != ""
}

// CanEdit: checks if a user can edit objects based on room permissions
func (r *Room) CanEdit(userID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Creator always has full permissions
	if r.CreatedBy == userID {
		return true
	}

	// If view-only mode, non-creators cannot edit
	if r.Permissions.ViewOnly {
		return false
	}

	return true
}

// CanEditObject: checks if a user can edit a specific object
func (r *Room) CanEditObject(userID string, objectID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Creator always has full permissions
	if r.CreatedBy == userID {
		return true
	}

	// If view-only mode, non-creators cannot edit
	if r.Permissions.ViewOnly {
		return false
	}

	// If OnlyEditOwn mode, check object ownership
	if r.Permissions.OnlyEditOwn {
		obj, exists := r.Objects[objectID]
		if !exists {
			return true // Allow creating new objects
		}
		return obj.UserID == userID
	}

	return true
}
