package util
 
import (
	"net/http"
	"strings"
)
 
// extracts the real client IP from the request
func GetClientIP(r *http.Request, behindProxy bool) string {

	if behindProxy {
		// Fly.io
		if flyIP := r.Header.Get("Fly-Client-IP"); flyIP != "" {
			return strings.TrimSpace(flyIP)
		}
 
		// Only trust the next 2 when behind a known proxy!
 
		// X-Forwarded-For header (set by proxies like nginx, Cloudflare)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				return strings.TrimSpace(ips[0])
			}
		}
 
		// Check X-Real-IP header (used by some proxies)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
 
	// Fallback to RemoteAddr (direct connection, no proxy)
	// RemoteAddr format: "ip:port" or "[ipv6]:port"
	ip := r.RemoteAddr
 
	// Handle IPv6: [2001:db8::1]:port
	if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]"); idx != -1 {
			return ip[:idx+1] // Keep brackets
		}
	}
 
	// Handle IPv4: 192.168.1.1:port
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		// IPv6 (multiple colons) vs IPv4:port (single colon)
		if strings.Count(ip[:idx], ":") == 0 {
			return ip[:idx] // IPv4, remove port
		}
	}
 
	// Return as-is (IPv6 without port, or malformed)
	return ip
}
