package handlers
 
import (
	"encoding/json"
	"fmt"
	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
	"time"
 
	"github.com/gorilla/websocket"
)
 
 
// Room creation with settings (password, permissions)
// Called when client sends createRoom message after authentication
func HandleCreateRoom(roomCode string, userID string, msgData map[string]interface{}, roomManager *room.Manager, rateLimit *middleware.RateLimit) error {
	logger.Info("Creating room with settings").
		Str("room_code", roomCode).
		Str("user_id", userID).
		Msg("")
 
	settings := &room.RoomSettings{
		CreatedBy: userID,
	}
 
	if password, ok := msgData["password"].(string); ok && password != "" {
		settings.Password = password
	}
 
	// Extract permissions if provided
	if perms, ok := msgData["permissions"].(map[string]interface{}); ok {
		if viewOnly, ok := perms["viewOnly"].(bool); ok {
			settings.Permissions.ViewOnly = viewOnly
		}
		if onlyEditOwn, ok := perms["onlyEditOwn"].(bool); ok {
			settings.Permissions.OnlyEditOwn = onlyEditOwn
		}
	}
 
	// Create room with settings
	_, err := roomManager.CreateRoomWithSettingsPublic(roomCode, settings, rateLimit)
	if err != nil {
		return fmt.Errorf("failed to create room: %w", err)
	}
 
	logger.Info("Room created successfully with settings").
		Str("room_code", roomCode).
		Msg("")
	return nil
}
 
// reads the client's intent (createRoom or joinRoom)
func HandleRoomIntent(conn *websocket.Conn, user *user.User, roomCode string, roomManager *room.Manager, rateLimit *middleware.RateLimit) (string, []byte, error) {
	// Client has 3 seconds to send intent message
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, nextMsg, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) // Reset deadline immediately
 
	intentType := "joinRoom" // Default to join if no message or timeout
 
	if err == nil {
		// Got a message -> parse intent
		var msgData map[string]interface{}
		if json.Unmarshal(nextMsg, &msgData) == nil {
			if msgType, ok := msgData["type"].(string); ok {
				intentType = msgType
 
				if msgType == "createRoom" {
					if err := HandleCreateRoom(roomCode, user.ID, msgData, roomManager, rateLimit); err != nil {
						logger.Error("Error creating room with settings").
							Err(err).
							Str("user_id", user.ID).
							Str("room_code", roomCode).
							Msg("")
						sendError(user, "ROOM_CREATE_FAILED", "Failed to create room: "+err.Error())
						return "", nil, fmt.Errorf("room creation failed: %w", err)
					}
				}
			}
		}
	} else {
		// Timeout or read error -> default to joinRoom behavior
		logger.Debug("No intent message received (timeout), defaulting to joinRoom").
			Str("user_id", user.ID).
			Msg("")
	}
 
	return intentType, nextMsg, nil
}
 
// HandleRoomJoin handles the logic of joining or creating a room
func HandleRoomJoin(user *user.User, session *user.UserSession, roomCode string, intentType string, roomManager *room.Manager, rateLimit *middleware.RateLimit) (*room.Room, error) {
	if intentType == "joinRoom" || intentType == "createRoom" {
		existingRoom, roomExists := roomManager.GetRoom(roomCode)
 
		// JOINING non-existent room
		if intentType == "joinRoom" && !roomExists {
			logger.Warn("User tried to join non-existent room").
				Str("user_id", user.ID).
				Str("room_code", roomCode).
				Msg("")
			sendError(user, "ROOM_NOT_FOUND", fmt.Sprintf("Room %s not found. Please check the room code.", roomCode))
			return nil, fmt.Errorf("room not found")
		}
 
		// Join room (will create if doesn't exist and was createRoom intent)
		var rm *room.Room
		var joinErr error
		if roomExists {
			rm = existingRoom
			joinErr = rm.Join(user, rateLimit.MaxRoomSize)
		} else {
			// createRoom intent, use JoinRoom to make it
			rm, joinErr = roomManager.JoinRoom(roomCode, session, user, rateLimit)
		}
 
		if joinErr != nil {
			logger.Error("Failed to join room").
				Str("room_code", roomCode).
				Err(joinErr).
				Msg("")
			sendError(user, "ROOM_JOIN_FAILED", "Failed to join room: "+joinErr.Error())
			return nil, fmt.Errorf("failed to join room: %w", joinErr)
		}
 
		return rm, nil
	}
 
	// Unknown intent type
	logger.Warn("Unknown intent type from user").
		Str("user_id", user.ID).
		Str("intent_type", intentType).
		Msg("")
	sendError(user, "INVALID_INTENT", "Invalid intent type")
	return nil, fmt.Errorf("invalid intent type")
}

func sendError(u *user.User, code string, message string) {
	errorResponse := map[string]interface{}{
		"type":    "error",
		"code":    code,
		"message": message,
	}
	errorMsg, err := json.Marshal(errorResponse)
	if err != nil {
		logger.Error("Failed to marshal error response").
			Err(err).
			Str("user_id", u.ID).
			Msg("")
		return
	}
	if err := u.WriteMessage(websocket.TextMessage, errorMsg); err != nil {
		logger.Error("Failed to send error response").
			Err(err).
			Str("user_id", u.ID).
			Msg("")
	}
}

