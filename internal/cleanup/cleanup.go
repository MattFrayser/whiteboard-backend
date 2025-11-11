package cleanup

import (
	"context"
	"time"

	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
)

// StartRoomCleanup periodically removes expired rooms
func StartRoomCleanup(ctx context.Context, roomMgr *room.Manager) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roomMgr.Cleanup()
			logger.Debug("Cleaned up expired rooms").Msg("")
		}
	}
}

// StartSessionCleanup periodically removes expired user sessions
func StartSessionCleanup(ctx context.Context, sessionMgr *user.SessionManager) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionMgr.Cleanup()
			logger.Debug("Cleaned up expired sessions").Msg("")
		}
	}
}

// StartIPLimiterCleanup periodically clears IP rate limiters
func StartIPLimiterCleanup(ctx context.Context, ipRateLimiter *middleware.IPRateLimit) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ipRateLimiter.Cleanup()
			logger.Debug("IP rate limiters cleared").Msg("")
		}
	}
}
