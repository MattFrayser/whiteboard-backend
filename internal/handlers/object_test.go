package handlers

import (
	"encoding/json"
	"testing"

	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"

	"github.com/gorilla/websocket"
)

// MockConn is a mock WebSocket connection for testing
type MockConn struct {
	messages [][]byte
}

func (m *MockConn) WriteMessage(messageType int, data []byte) error {
	m.messages = append(m.messages, data)
	return nil
}

func (m *MockConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, nil
}

func (m *MockConn) Close() error {
	return nil
}

func TestHandleAdded_SendsAcknowledgment(t *testing.T) {
	// Setup
	validator := object.NewValidator()
	config := &middleware.RateLimit{}
	broadcaster := room.NewBroadcaster()
	handler := NewObjectHandler(validator, config, broadcaster)

	rm := room.NewRoom("test-room", "", false)
	mockConn := &MockConn{messages: make([][]byte, 0)}
	u := &user.User{
		ID: "test-user",
		Session: &user.UserSession{
			UserID: "test-user",
		},
		Connection: (*websocket.Conn)(nil), // Will use WriteMessage directly
	}

	// Mock the WriteMessage call
	originalWriteMessage := u.WriteMessage
	var capturedAck map[string]interface{}
	u.WriteMessage = func(messageType int, data []byte) error {
		json.Unmarshal(data, &capturedAck)
		return mockConn.WriteMessage(messageType, data)
	}
	defer func() { u.WriteMessage = originalWriteMessage }()

	// Test data
	data := map[string]interface{}{
		"type": "objectAdded",
		"object": map[string]interface{}{
			"id":     "obj-123",
			"type":   "rectangle",
			"zIndex": float64(1),
			"data": map[string]interface{}{
				"x1":    float64(0),
				"y1":    float64(0),
				"x2":    float64(100),
				"y2":    float64(100),
				"color": "#000000",
				"width": float64(2),
			},
		},
	}

	// Execute
	err := handler.HandleAdded(rm, u, data)

	// Assert
	if err != nil {
		t.Fatalf("HandleAdded returned error: %v", err)
	}

	// Verify acknowledgment was sent
	if len(mockConn.messages) == 0 {
		t.Fatal("No acknowledgment message was sent")
	}

	if capturedAck == nil {
		t.Fatal("Failed to capture acknowledgment")
	}

	if capturedAck["type"] != "objectAdded_ack" {
		t.Errorf("Expected ack type 'objectAdded_ack', got %v", capturedAck["type"])
	}

	if capturedAck["objectId"] != "obj-123" {
		t.Errorf("Expected objectId 'obj-123', got %v", capturedAck["objectId"])
	}

	if capturedAck["success"] != true {
		t.Errorf("Expected success true, got %v", capturedAck["success"])
	}
}

func TestHandleAdded_SendsErrorAck_OnPermissionDenied(t *testing.T) {
	// Setup
	validator := object.NewValidator()
	config := &middleware.RateLimit{}
	broadcaster := room.NewBroadcaster()
	handler := NewObjectHandler(validator, config, broadcaster)

	// Create view-only room
	rm := room.NewRoom("test-room", "", true) // true = view-only
	mockConn := &MockConn{messages: make([][]byte, 0)}
	u := &user.User{
		ID: "test-user",
		Session: &user.UserSession{
			UserID: "test-user",
		},
		Connection: (*websocket.Conn)(nil),
	}

	// Mock the WriteMessage call
	var capturedAck map[string]interface{}
	u.WriteMessage = func(messageType int, data []byte) error {
		json.Unmarshal(data, &capturedAck)
		return mockConn.WriteMessage(messageType, data)
	}

	// Test data
	data := map[string]interface{}{
		"type": "objectAdded",
		"object": map[string]interface{}{
			"id":     "obj-123",
			"type":   "rectangle",
			"zIndex": float64(1),
			"data": map[string]interface{}{
				"x1":    float64(0),
				"y1":    float64(0),
				"x2":    float64(100),
				"y2":    float64(100),
				"color": "#000000",
				"width": float64(2),
			},
		},
	}

	// Execute
	err := handler.HandleAdded(rm, u, data)

	// Assert - should return error
	if err == nil {
		t.Fatal("Expected error for view-only room, got nil")
	}

	// Verify error acknowledgment was sent
	if len(mockConn.messages) == 0 {
		t.Fatal("No error acknowledgment message was sent")
	}

	if capturedAck["type"] != "objectAdded_error" {
		t.Errorf("Expected ack type 'objectAdded_error', got %v", capturedAck["type"])
	}

	if capturedAck["success"] != false {
		t.Errorf("Expected success false, got %v", capturedAck["success"])
	}

	if capturedAck["error"] == nil {
		t.Error("Expected error message in acknowledgment")
	}
}

func TestHandleAdded_SendsErrorAck_OnInvalidObjectType(t *testing.T) {
	// Setup
	validator := object.NewValidator()
	config := &middleware.RateLimit{}
	broadcaster := room.NewBroadcaster()
	handler := NewObjectHandler(validator, config, broadcaster)

	rm := room.NewRoom("test-room", "", false)
	mockConn := &MockConn{messages: make([][]byte, 0)}
	u := &user.User{
		ID: "test-user",
		Session: &user.UserSession{
			UserID: "test-user",
		},
		Connection: (*websocket.Conn)(nil),
	}

	// Mock the WriteMessage call
	var capturedAck map[string]interface{}
	u.WriteMessage = func(messageType int, data []byte) error {
		json.Unmarshal(data, &capturedAck)
		return mockConn.WriteMessage(messageType, data)
	}

	// Test data with invalid object type
	data := map[string]interface{}{
		"type": "objectAdded",
		"object": map[string]interface{}{
			"id":     "obj-123",
			"type":   "invalid-type", // Invalid type
			"zIndex": float64(1),
			"data": map[string]interface{}{
				"x1": float64(0),
				"y1": float64(0),
			},
		},
	}

	// Execute
	err := handler.HandleAdded(rm, u, data)

	// Assert
	if err == nil {
		t.Fatal("Expected error for invalid object type, got nil")
	}

	// Verify error acknowledgment was sent
	if capturedAck["type"] != "objectAdded_error" {
		t.Errorf("Expected ack type 'objectAdded_error', got %v", capturedAck["type"])
	}
}
