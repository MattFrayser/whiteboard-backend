# Logging Strategy

## Overview

The backend uses [zerolog](https://github.com/rs/zerolog) for structured logging with environment-based configuration.

## Configuration

Set via environment variables in `.env`:

```bash
ENVIRONMENT=development  # or "production"
LOG_LEVEL=debug         # debug, info, warn, error
```

**Development mode**: Pretty console output with colors
**Production mode**: JSON-formatted logs to stdout for aggregation

## Log Levels

Only log what matters:

- **ERROR**: Failed operations, unrecoverable errors
- **WARN**: Recoverable issues, security events, misconfigurations
- **INFO**: Important state changes (server start/stop, shutdown sequence)
- **DEBUG**: Development-only troubleshooting (disabled in production)

## What We Log

✅ **DO log:**
- Errors and failures
- Security events (fingerprint mismatch, rate limiting)
- Server lifecycle (startup, shutdown, connection counts)
- TLS/production warnings

❌ **DON'T log:**
- Routine successful operations ("user authenticated", "message sent")
- Normal flow events ("connection registered", "room created")
- Data echoing (redundant state information)

## Example Usage

```go
import "main/internal/logger"

// Error with context
logger.Error("Failed to authenticate connection").
    Err(err).
    Str("room_code", roomCode).
    Msg("")

// Warning for security events
logger.Warn("Fingerprint mismatch - generating new session").
    Str("user_id", validUserID).
    Msg("")

// Info for important state changes
logger.Info("Server started").
    Str("addr", addr).
    Str("protocol", "HTTP/WS").
    Msg("")
```

## Migration Summary

Migrated from `log.Printf()` to zerolog across all backend files:
- main.go (20 statements)
- websocket.go (57 statements → reduced to ~42 after cleanup)
- connection_registry.go (5 statements → 3 after cleanup)
- authenticator.go (2 statements → 0 after cleanup)
- object.go (4 statements)
- sync.go (1 statement → 0 after cleanup)
- broadcaster.go (1 statement)

**Total**: 66 log statements migrated, ~20 redundant debug logs removed

## Production Considerations

1. **TLS Enforcement**: Server will refuse to start in production without TLS certificates
2. **Log Aggregation**: JSON output can be piped to logging services (CloudWatch, Datadog, etc.)
3. **Performance**: Zerolog is one of the fastest Go logging libraries (zero allocation)
4. **Sensitive Data**: Never log session tokens, user IDs in production (use debug level only)
