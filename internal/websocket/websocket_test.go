package transport

import (
	"net/http"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xForwardedFor  string
		xRealIP        string
		expectedIP     string
	}{
		{
			name:       "Direct connection without proxy",
			remoteAddr: "192.168.1.100:54321",
			expectedIP: "192.168.1.100",
		},
		{
			name:          "X-Forwarded-For with single IP",
			remoteAddr:    "10.0.0.1:54321",
			xForwardedFor: "203.0.113.45",
			expectedIP:    "203.0.113.45",
		},
		{
			name:          "X-Forwarded-For with multiple IPs (proxy chain)",
			remoteAddr:    "10.0.0.1:54321",
			xForwardedFor: "203.0.113.45, 198.51.100.10, 192.0.2.5",
			expectedIP:    "203.0.113.45",
		},
		{
			name:          "X-Forwarded-For with spaces",
			remoteAddr:    "10.0.0.1:54321",
			xForwardedFor: "  203.0.113.45  , 198.51.100.10",
			expectedIP:    "203.0.113.45",
		},
		{
			name:       "X-Real-IP header",
			remoteAddr: "10.0.0.1:54321",
			xRealIP:    "203.0.113.45",
			expectedIP: "203.0.113.45",
		},
		{
			name:          "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr:    "10.0.0.1:54321",
			xForwardedFor: "203.0.113.45",
			xRealIP:       "198.51.100.10",
			expectedIP:    "203.0.113.45",
		},
		{
			name:       "IPv6 address",
			remoteAddr: "[2001:db8::1]:54321",
			expectedIP: "[2001:db8::1]",
		},
		{
			name:          "X-Forwarded-For with IPv6",
			remoteAddr:    "[::1]:54321",
			xForwardedFor: "2001:db8::1",
			expectedIP:    "2001:db8::1",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "192.168.1.100",
			expectedIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP request
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Set RemoteAddr
			req.RemoteAddr = tt.remoteAddr

			// Set headers if provided
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			// Test GetClientIP
			got := GetClientIP(req)
			if got != tt.expectedIP {
				t.Errorf("GetClientIP() = %v, want %v", got, tt.expectedIP)
			}
		})
	}
}
