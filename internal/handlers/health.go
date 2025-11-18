package handlers

import (
	"encoding/json"
	"net/http"

	"main/internal/logger"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
}

// HandleHealth provides a simple health check for infrastructure monitoring
// This is intended for Fly.io, load balancers, and monitoring systems
func HandleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Simple 200 OK response - server is alive and responding
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := HealthResponse{
			Status: "healthy",
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode health response").
				Err(err).
				Msg("")
		}
	}
}
