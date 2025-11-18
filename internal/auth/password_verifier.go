package auth
 
import (
	"encoding/json"
	"fmt"
	"main/internal/logger"
	"main/internal/room"
	"main/internal/user"
	"time"
 
	"github.com/gorilla/websocket"
)
 
const (
	PasswordEntryTimeout        = 30 * time.Second
	MaxPasswordAttempts         = 3
	SessionVerificationDuration = 1 * time.Hour
)
 
// Automatically resets the deadline after reading (or on error)
func readMessageWithTimeout(conn *websocket.Conn, timeout time.Duration) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{}) // Always reset deadline
 
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}
	return msg, nil
}
 
// parse JSON message and extracts the password field
func extractPasswordFromMessage(msg []byte) (string, error) {
	if msg == nil {
		return "", nil
	}
 
	var msgData map[string]interface{}
	if err := json.Unmarshal(msg, &msgData); err != nil {
		return "", fmt.Errorf("failed to parse message: %w", err)
	}
 
	if pwd, ok := msgData["password"].(string); ok {
		return pwd, nil
	}
 
	return "", nil
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
 
type PasswordVerifier struct {
	conn     *websocket.Conn
	user     *user.User
	room     *room.Room
	session  *user.UserSession
	roomCode string
	timeout  time.Duration
}
 
func NewPasswordVerifier(conn *websocket.Conn, user *user.User, room *room.Room, session *user.UserSession, roomCode string) *PasswordVerifier {
	return &PasswordVerifier{
		conn:     conn,
		user:     user,
		room:     room,
		session:  session,
		roomCode: roomCode,
		timeout:  PasswordEntryTimeout,
	}
}
 
// check if the user already verified password for this room
func (pv *PasswordVerifier) isVerifiedInSession() bool {
	verifiedTime, exists := pv.session.VerifiedRooms[pv.roomCode]
	if !exists {
		return false
	}
 
	// verification is still valid
	if time.Since(verifiedTime) < SessionVerificationDuration {
		return true
	}
 
	// Verification expired, remove it
	delete(pv.session.VerifiedRooms, pv.roomCode)
	return false
}
 
// prompt user for password and await response
func (pv *PasswordVerifier) requestPassword(errorCode, errorMessage string) (string, error) {
	sendError(pv.user, errorCode, errorMessage)
 
	msg, err := readMessageWithTimeout(pv.conn, pv.timeout)
	if err != nil {
		logger.Warn("Timeout waiting for password").
			Str("user_id", pv.user.ID).
			Err(err).
			Msg("")
		sendError(pv.user, "AUTH_TIMEOUT", "Password entry timeout")
		return "", fmt.Errorf("password entry timeout: %w", err)
	}
 
	password, err := extractPasswordFromMessage(msg)
	if err != nil {
		logger.Warn("Failed to parse password message").
			Str("user_id", pv.user.ID).
			Err(err).
			Msg("")
		return "", fmt.Errorf("invalid password message: %w", err)
	}
 
	return password, nil
}
 
func (pv *PasswordVerifier) verifyAttempt(password string) bool {
	return VerifyPassword(pv.room.Password, password)
}
 
// stores successful verification in session
func (pv *PasswordVerifier) markVerified() {
	pv.session.VerifiedRooms[pv.roomCode] = time.Now()
	logger.Info("Password verified for room").
		Str("user_id", pv.user.ID).
		Str("room_code", pv.roomCode).
		Msg("")
}
 
// orchestrates password verification process
func (pv *PasswordVerifier) Verify(initialMsg []byte) error {
	// Already verified in session
	if pv.isVerifiedInSession() {
		return nil
	}
 
	password, _ := extractPasswordFromMessage(initialMsg)
 
	// loop allows for multiple attemps 
	for attempt := 0; attempt < MaxPasswordAttempts; attempt++ {
		if password == "" {
			logger.Warn("No password provided for protected room").
				Str("user_id", pv.user.ID).
				Str("room_code", pv.roomCode).
				Int("attempt", attempt+1).
				Msg("")
 
			var err error
			password, err = pv.requestPassword("PASSWORD_REQUIRED", "This room requires a password")
			if err != nil {
				return err
			}
			continue
		}
 
		if !pv.verifyAttempt(password) {
			logger.Warn("Invalid password for room").
				Str("user_id", pv.user.ID).
				Str("room_code", pv.roomCode).
				Int("attempt", attempt+1).
				Int("max_attempts", MaxPasswordAttempts).
				Msg("")
 
			if attempt+1 >= MaxPasswordAttempts {
				logger.Warn("Maximum password attempts exceeded").
					Str("user_id", pv.user.ID).
					Str("room_code", pv.roomCode).
					Msg("")
				sendError(pv.user, "MAX_ATTEMPTS_EXCEEDED", "Maximum password attempts exceeded")
				return fmt.Errorf("max password attempts exceeded")
			}
 
			// Request retry
			var err error
			password, err = pv.requestPassword("INVALID_PASSWORD", "Incorrect password")
			if err != nil {
				return err
			}
			continue
		}
 
		pv.markVerified()
		return nil
	}
 
	return fmt.Errorf("password verification failed")
}
 
// convenience wrapper for password verification
func VerifyRoomPassword(conn *websocket.Conn, room *room.Room, session *user.UserSession, roomCode string, user *user.User, nextMsg []byte) error {
	verifier := NewPasswordVerifier(conn, user, room, session, roomCode)
	return verifier.Verify(nextMsg)
}
