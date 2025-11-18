package middleware

import (
	"sort"
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
	limiters   map[string]*ipLimiterEntry
	mu         sync.RWMutex
	maxEntries int
}

// NewIPRateLimit: creates a new IPRateLimit
func NewIPRateLimit() *IPRateLimit {
	return &IPRateLimit{
		limiters: make(map[string]*ipLimiterEntry),
		maxEntries: 10000, 
	}
}

// Allow: checks if an IP is allowed to make a request
func (iprl *IPRateLimit) Allow(ip string) bool {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	entry, exists := iprl.limiters[ip]
	if !exists {
		if len(iprl.limiters) >= iprl.maxEntries {
			iprl.evictOldest()
		}

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

// remove 10% when full
func (iprl *IPRateLimit) evictOldest() {
	toRemove := iprl.maxEntries / 10
	
	type entry struct {
		ip       string
		lastSeen time.Time
	}
	entries := make([]entry, 0, len(iprl.limiters))
	
	for ip, e := range iprl.limiters {
		entries = append(entries, entry{ip: ip, lastSeen: e.lastSeen})
	}
	
	// Sort by lastSeen (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastSeen.Before(entries[j].lastSeen)
	})
	
	// Remove oldest 10%
	for i := 0; i < toRemove && i < len(entries); i++ {
		delete(iprl.limiters, entries[i].ip)
	}
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
