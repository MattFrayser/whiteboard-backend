package handlers

import (
	"encoding/json"
	"fmt"

	"main/internal/user"

	"github.com/gorilla/websocket"
)

type UserHandler struct{}

func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// HandleGetUserID: processes getUserId messages and returns the user ID
func (h *UserHandler) HandleGetUserID(u *user.User) error {
	response := map[string]interface{}{
		"type":   "userId",
		"userId": u.ID,
	}

	responseMsg, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal user ID response: %w", err)
	}

	return u.WriteMessage(websocket.TextMessage, responseMsg)
}
