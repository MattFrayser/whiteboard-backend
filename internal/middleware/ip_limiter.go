package middleware

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiterEntry: tracks a rate limiter and its last use time
type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimit: manages rate limiters per IP address
type IPRateLimit struct {
	limiters map[string]*ipLimiterEntry
	mu       sync.RWMutex
}

// NewIPRateLimit: creates a new IPRateLimit
func NewIPRateLimit() *IPRateLimit {
	return &IPRateLimit{
		limiters: make(map[string]*ipLimiterEntry),
	}
}

// Allow: checks if an IP is allowed to make a request
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

// Cleanup: removes old IP limiters that haven't been used recently
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
