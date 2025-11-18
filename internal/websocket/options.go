package transport

import (
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
)

// WebSocketConfigOption is a function that configures a WebSocketConfig
type WebSocketConfigOption func(*WebSocketConfig)

// WithIPRateLimiter sets the IP rate limiter for WebSocket connections
func WithIPRateLimiter(limiter *middleware.IPRateLimit) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.IPRateLimiter = limiter
	}
}

// WithRateLimit sets the rate limit configuration for WebSocket messages
func WithRateLimit(limit *middleware.RateLimit) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.RateLimit = limit
	}
}

// WithValidator sets the object validator for validating drawing objects
func WithValidator(validator *object.Validator) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.Validator = validator
	}
}

// WithSynchronizer sets the room synchronizer for object persistence
func WithSynchronizer(sync *room.Synchronizer) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.Synchronizer = sync
	}
}

// WithAuthenticator sets the WebSocket authenticator for connection authentication
func WithAuthenticator(auth *Authenticator) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.Authenticator = auth
	}
}

// WithConnectionTracker sets the connection tracker for monitoring active connections
func WithConnectionTracker(tracker *middleware.ConnectionTracker) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.ConnTracker = tracker
	}
}

// WithConnectionRegistry sets the connection registry for managing user connections
func WithConnectionRegistry(registry *ConnectionRegistry) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.ConnRegistry = registry
	}
}

func WithBehindProxy(behindProxy bool) WebSocketConfigOption {
	return func(c *WebSocketConfig) {
		c.BehindProxy = behindProxy
	}
}
