package middleware

import (
	"sync"
	"time"

	"main/internal/domain"

	"golang.org/x/time/rate"
)

// RateLimit holds configuration for rate limiting
type RateLimit struct {
	MaxRoomSize       int
	MaxObjects        int
	MaxMessageSize    int
	MaxRooms          int
	MessagesPerSecond float64
	BurstSize         int
}

// IPRateLimit manages rate limiters per IP address
type IPRateLimit struct {
	Limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

// NewRateLimit creates a new RateLimit configuration
func NewRateLimit(maxRoomSize, maxObjects, maxMessageSize, maxRooms int, messagesPerSecond float64, burstSize int) *RateLimit {
	return &RateLimit{
		MaxRoomSize:       maxRoomSize,
		MaxObjects:        maxObjects,
		MaxMessageSize:    maxMessageSize,
		MaxRooms:          maxRooms,
		MessagesPerSecond: messagesPerSecond,
		BurstSize:         burstSize,
	}
}

// NewIPRateLimit creates a new IPRateLimit
func NewIPRateLimit() *IPRateLimit {
	return &IPRateLimit{
		Limiters: make(map[string]*rate.Limiter),
	}
}

// CanAddObject checks if a room has space for more objects
func (rl *RateLimit) CanAddObject(room *domain.Room) bool {
	return room.GetObjectCount() < rl.MaxObjects
}

// ValidateMessageSize checks if a message is within the size limit
func (rl *RateLimit) ValidateMessageSize(msgSize int) bool {
	return msgSize <= rl.MaxMessageSize
}

// Allow checks if an IP is allowed to make a request
func (iprl *IPRateLimit) Allow(ip string) bool {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	limiter, exists := iprl.Limiters[ip]
	if !exists {
		// New IP: 10 connections per minute, burst of 5
		limiter = rate.NewLimiter(rate.Every(6*time.Second), 5)
		iprl.Limiters[ip] = limiter
	}

	return limiter.Allow()
}

// Cleanup removes old IP limiters
func (iprl *IPRateLimit) Cleanup() {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	// Clear all limiters (they'll be recreated on demand)
	// In production, you'd track last use time and only remove old ones
	iprl.Limiters = make(map[string]*rate.Limiter)
}
