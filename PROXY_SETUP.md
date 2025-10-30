# Proxy Configuration Guide

The whiteboard server now properly handles client IP extraction when deployed behind a reverse proxy (nginx, Cloudflare, etc.).

## How It Works

The `GetClientIP()` function checks headers in this order:

1. **X-Forwarded-For** - Used by most proxies (nginx, Cloudflare, HAProxy)
2. **X-Real-IP** - Alternative header used by some proxies
3. **RemoteAddr** - Fallback for direct connections (no proxy)

## Nginx Configuration

Add these headers to your nginx config to pass the real client IP:

```nginx
location /ws {
    proxy_pass http://localhost:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "Upgrade";
    proxy_set_header Host $host;

    # Pass real client IP
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location / {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

## Apache Configuration

```apache
<VirtualHost *:443>
    ProxyPass /ws ws://localhost:8080/ws
    ProxyPassReverse /ws ws://localhost:8080/ws

    ProxyPass / http://localhost:8080/
    ProxyPassReverse / http://localhost:8080/

    # Pass real client IP
    RequestHeader set X-Forwarded-Proto "https"
    RequestHeader set X-Real-IP %{REMOTE_ADDR}s
</VirtualHost>
```

## Cloudflare

Cloudflare automatically sets these headers:
- `CF-Connecting-IP` - The real client IP
- `X-Forwarded-For` - Chain of IPs

The server will use `X-Forwarded-For` which Cloudflare provides.

**Important**: Enable "IP Geolocation" in Cloudflare dashboard for accurate IP forwarding.

## Security Considerations

### ⚠️ Only Trust Proxies You Control

The server trusts the **first IP** in `X-Forwarded-For`. This is safe when:
- ✅ You control the proxy (nginx on your server)
- ✅ You use a trusted CDN (Cloudflare, CloudFront)
- ❌ **DANGEROUS**: Accepting connections directly from internet with X-Forwarded-For

### Why This Matters

If an attacker can connect directly to your server (bypassing your proxy), they can set:
```
X-Forwarded-For: 1.2.3.4
```

And the server will think they're connecting from `1.2.3.4`, bypassing rate limiting.

### How to Protect

**Option 1**: Firewall rules (recommended)
```bash
# Only allow connections from your proxy
sudo ufw allow from 10.0.0.1 to any port 8080  # Your nginx server
sudo ufw deny 8080
```

**Option 2**: Listen on localhost only
```go
// In main.go, change:
http.ListenAndServe(":8080", nil)
// To:
http.ListenAndServe("127.0.0.1:8080", nil)
```

**Option 3**: Configure trusted proxies (code modification needed)
```go
// Add validation that request came from known proxy
func GetClientIP(r *http.Request) string {
    // Only trust X-Forwarded-For from known proxies
    remoteAddr := r.RemoteAddr
    if !isTrustedProxy(remoteAddr) {
        // Don't trust headers, use direct connection IP
        return extractIP(remoteAddr)
    }

    // ... rest of existing logic
}
```

## Testing

Test that IP extraction works correctly:

```bash
# Run unit tests
cd backend
go test ./internal/websocket -v

# Test with curl (should see your real IP in logs)
curl -H "Origin: http://localhost:5173" \
     -H "X-Forwarded-For: 203.0.113.45" \
     http://localhost:8080/

# Check server logs for:
# "WebSocket connection accepted from origin: ..."
```

## Rate Limiting Impact

With proper IP extraction:
- ✅ Each client has their own rate limit (10 connections/min)
- ✅ Rate limiting works correctly behind proxies
- ✅ Abusive clients are blocked by their real IP

Without proper IP extraction:
- ❌ All clients share the proxy's rate limit
- ❌ One abusive client can block everyone
- ❌ Rate limiting is essentially disabled

## Origin Validation

The server now logs all connection attempts:

**Accepted connection:**
```
WebSocket connection accepted from origin: https://yourdomain.com
```

**Rejected connection:**
```
WebSocket connection rejected from unauthorized origin: https://evil.com (allowed: [https://yourdomain.com])
```

Monitor these logs to detect:
- Misconfigured clients (legitimate origin not in allowlist)
- Attack attempts (suspicious origins)
- Development issues (localhost connections in production)

## Production Checklist

- [ ] Configure nginx/Apache to set X-Forwarded-For header
- [ ] Update `.env` with production domains: `DOMAINS="https://yourdomain.com"`
- [ ] Enable TLS: Set `TLS_CERT_FILE` and `TLS_KEY_FILE` in `.env`
- [ ] Firewall server to only accept connections from proxy
- [ ] Test IP extraction with curl/browser dev tools
- [ ] Monitor logs for rejected origins
- [ ] Verify rate limiting works per-client
