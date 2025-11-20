package tests

import (
	"testing"
	"time"

	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"
)

func TestManager_CreateAndJoinRoom(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 50}

	session := &user.UserSession{
		UserID:   "user-1",
		LastRoom: "",
	}
	u := &user.User{
		ID:      "user-1",
		Session: session,
	}

	// Create room by joining (room code must be exactly 6 characters)
	rm, err := manager.JoinRoom("room01", session, u, rl)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	if rm == nil {
		t.Fatal("Room should not be nil")
	}

	if rm.ConnectionCount() != 1 {
		t.Errorf("Expected 1 connection, got %d", rm.ConnectionCount())
	}

	// Check user was assigned a color
	color := rm.GetUserColor("user-1")
	if color == "" {
		t.Error("User should have been assigned a color")
	}

	// Verify user is creator (first joiner)
	if !rm.IsCreator("user-1") {
		t.Error("First user should be creator")
	}
}

func TestManager_RoomCapacityLimit(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 1, MaxRoomSize: 50}

	session1 := &user.UserSession{UserID: "user-1"}
	u1 := &user.User{ID: "user-1", Session: session1}

	// Create first room (room code must be exactly 6 characters)
	_, err := manager.JoinRoom("room01", session1, u1, rl)
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Try to create second room (should fail - at capacity)
	session2 := &user.UserSession{UserID: "user-2"}
	u2 := &user.User{ID: "user-2", Session: session2}

	_, err = manager.JoinRoom("room02", session2, u2, rl)
	if err == nil {
		t.Fatal("Expected error when exceeding room capacity")
	}
	if err.Error() != "server at maximum room capacity" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestManager_RoomSizeLimit(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 2}

	// Add first user (room code must be exactly 6 characters)
	session1 := &user.UserSession{UserID: "user-1"}
	u1 := &user.User{ID: "user-1", Session: session1}
	rm, _ := manager.JoinRoom("room03", session1, u1, rl)

	// Add second user
	session2 := &user.UserSession{UserID: "user-2"}
	u2 := &user.User{ID: "user-2", Session: session2}
	_, err := manager.JoinRoom("room03", session2, u2, rl)
	if err != nil {
		t.Fatalf("Failed to add second user: %v", err)
	}

	if rm.ConnectionCount() != 2 {
		t.Errorf("Expected 2 connections, got %d", rm.ConnectionCount())
	}

	// Try to add third user (should fail)
	session3 := &user.UserSession{UserID: "user-3"}
	u3 := &user.User{ID: "user-3", Session: session3}
	_, err = manager.JoinRoom("room03", session3, u3, rl)
	if err == nil {
		t.Fatal("Expected error when room is full")
	}
}

func TestRoom_ViewOnlyPermissions(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 50}

	// Create view-only room (room code must be exactly 6 characters)
	settings := &room.RoomSettings{
		Permissions: room.RoomPermissions{ViewOnly: true},
		CreatedBy:   "creator-id",
	}
	rm, err := manager.CreateRoomWithSettingsPublic("view01", settings, rl)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Creator can edit
	if !rm.CanEdit("creator-id") {
		t.Error("Creator should be able to edit in view-only room")
	}

	// Non-creator cannot edit
	if rm.CanEdit("other-user") {
		t.Error("Non-creator should not be able to edit in view-only room")
	}
}

func TestRoom_OnlyEditOwnPermissions(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 50}

	// Create edit-own room (room code must be exactly 6 characters)
	settings := &room.RoomSettings{
		Permissions: room.RoomPermissions{OnlyEditOwn: true},
		CreatedBy:   "creator-id",
	}
	rm, err := manager.CreateRoomWithSettingsPublic("edit01", settings, rl)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Add object by user1
	obj := &object.Drawing{
		ID:     "obj-1",
		UserID: "user-1",
	}
	rm.AddObject(obj)

	// User1 can edit their own object
	if !rm.CanEditObject("user-1", "obj-1") {
		t.Error("User should be able to edit their own object")
	}

	// User2 cannot edit user1's object
	if rm.CanEditObject("user-2", "obj-1") {
		t.Error("User should not be able to edit other user's object")
	}

	// Creator can edit any object
	if !rm.CanEditObject("creator-id", "obj-1") {
		t.Error("Creator should be able to edit any object")
	}

	// Users can create new objects (non-existent ID)
	if !rm.CanEditObject("user-2", "new-obj") {
		t.Error("User should be able to create new objects")
	}
}

func TestManager_RoomCleanup(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 50}

	session := &user.UserSession{UserID: "user-1"}
	u := &user.User{ID: "user-1", Session: session}

	// Create room (room code must be exactly 6 characters)
	rm, _ := manager.JoinRoom("clean1", session, u, rl)

	// Remove user (room becomes empty)
	rm.Leave(u)

	// Set LastActive to over 1 hour ago
	rm.LastActive = time.Now().Add(-2 * time.Hour)

	// Run cleanup
	manager.Cleanup()

	// Room should be removed
	_, exists := manager.GetRoom("clean1")
	if exists {
		t.Error("Empty inactive room should have been cleaned up")
	}
}

func TestManager_InvalidRoomCode(t *testing.T) {
	manager := room.NewManager()
	rl := &middleware.RateLimit{MaxRooms: 10, MaxRoomSize: 50}

	session := &user.UserSession{UserID: "user-1"}
	u := &user.User{ID: "user-1", Session: session}

	// Try invalid characters (6 chars but with special characters)
	_, err := manager.JoinRoom("room@#", session, u, rl)
	if err == nil {
		t.Error("Should reject room code with special characters")
	}

	// Try invalid length (too short)
	_, err = manager.JoinRoom("abc", session, u, rl)
	if err == nil {
		t.Error("Should reject room code with wrong length")
	}

	// Try invalid length (too long)
	_, err = manager.JoinRoom("abcdefg", session, u, rl)
	if err == nil {
		t.Error("Should reject room code with wrong length")
	}
}
