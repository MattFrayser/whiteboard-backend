package handlers

import (
	"encoding/json"
	"net/http"

	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
)

// ConnectionCounter interface for counting active connections
type ConnectionCounter interface {
	Count() int
}

// HealthConfig holds dependencies and thresholds for health checks
type HealthConfig struct {
	SessionMgr     *user.SessionManager
	ConnRegistry   ConnectionCounter
	RoomMgr        *room.Manager
	ConnTracker    *middleware.ConnectionTracker
	MaxConnections int
	MaxRooms       int
	MaxHTTPConns   int
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// HandleHealth performs authenticated health checks with threshold validation
func HandleHealth(cfg *HealthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Require session authentication
		cookie, err := r.Cookie("whiteboard_session_token")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate session token
		_, valid := cfg.SessionMgr.ValidateToken(cookie.Value)
		if !valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check component health against thresholds
		wsCount := cfg.ConnRegistry.Count()
		roomCount := cfg.RoomMgr.RoomCount()
		httpCount := cfg.ConnTracker.Count()

		// Determine health status
		var status string
		var reason string
		statusCode := http.StatusOK

		if wsCount > cfg.MaxConnections {
			status = "unhealthy"
			reason = "WebSocket connections exceeded threshold"
			statusCode = http.StatusServiceUnavailable
		} else if roomCount > cfg.MaxRooms {
			status = "unhealthy"
			reason = "Active rooms exceeded threshold"
			statusCode = http.StatusServiceUnavailable
		} else if httpCount > cfg.MaxHTTPConns {
			status = "unhealthy"
			reason = "Concurrent HTTP connections exceeded threshold"
			statusCode = http.StatusServiceUnavailable
		} else {
			status = "healthy"
		}

		// Log unhealthy state
		if status == "unhealthy" {
			logger.Warn("Health check failed").
				Str("reason", reason).
				Int("ws_connections", wsCount).
				Int("rooms", roomCount).
				Int("http_connections", httpCount).
				Msg("")
		}

		// Return JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		response := HealthResponse{
			Status: status,
			Reason: reason,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode health response").
				Err(err).
				Msg("")
		}
	}
}
