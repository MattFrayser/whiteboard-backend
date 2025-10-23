package middleware

import (
	"sync"
	"time"

	"main/internal/room"

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

// ipLimiterEntry tracks a rate limiter and its last use time
type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimit manages rate limiters per IP address
type IPRateLimit struct {
	limiters map[string]*ipLimiterEntry
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
		limiters: make(map[string]*ipLimiterEntry),
	}
}

// CanAddObject checks if a room has space for more objects
func (rl *RateLimit) CanAddObject(rm *room.Room) bool {
	return rm.GetObjectCount() < rl.MaxObjects
}

// ValidateMessageSize checks if a message is within the size limit
func (rl *RateLimit) ValidateMessageSize(msgSize int) bool {
	return msgSize <= rl.MaxMessageSize
}

// Allow checks if an IP is allowed to make a request
func (iprl *IPRateLimit) Allow(ip string) bool {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	entry, exists := iprl.limiters[ip]
	if !exists {
		// New IP: 10 connections per minute, burst of 5
		entry = &ipLimiterEntry{
			limiter:  rate.NewLimiter(rate.Every(6*time.Second), 5),
			lastSeen: time.Now(),
		}
		iprl.limiters[ip] = entry
	} else {
		// Update last seen time
		entry.lastSeen = time.Now()
	}

	return entry.limiter.Allow()
}

// Cleanup removes old IP limiters that haven't been used recently
func (iprl *IPRateLimit) Cleanup() {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	now := time.Now()
	threshold := 1 * time.Hour

	for ip, entry := range iprl.limiters {
		if now.Sub(entry.lastSeen) > threshold {
			delete(iprl.limiters, ip)
		}
	}
}
