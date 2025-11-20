package tests

import (
	"sync"
	"testing"

	"main/internal/room"
	"main/internal/user"
)

// The broadcaster is tightly coupled to gorilla/websocket which is difficult to mock
// Basic tests ensure the broadcaster can be created and handles edge cases

type mockRoomForBroadcast struct {
	connections []*user.User
	removed     []string
	colors      map[string]string
	mu          sync.Mutex
}

func (m *mockRoomForBroadcast) GetConnections() []*user.User {
	return m.connections
}

func (m *mockRoomForBroadcast) RemoveConnection(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, userID)
}

func (m *mockRoomForBroadcast) GetUserColor(userID string) string {
	return m.colors[userID]
}

func TestBroadcaster_NewBroadcaster(t *testing.T) {
	broadcaster := room.NewBroadcaster()

	if broadcaster == nil {
		t.Error("NewBroadcaster should return non-nil broadcaster")
	}
}

func TestBroadcaster_EmptyRoom(t *testing.T) {
	broadcaster := room.NewBroadcaster()

	mockRoom := &mockRoomForBroadcast{
		connections: []*user.User{},
	}

	message := []byte(`{"type":"test"}`)

	// Should not panic on empty room
	// Test verifies code doesn't panic with empty input
	broadcaster.Broadcast(mockRoom, message, nil)
}

