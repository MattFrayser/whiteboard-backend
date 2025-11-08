package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var Logger zerolog.Logger

// Init initializes the global logger with environment-based configuration
func Init() {
	// Determine environment
	environment := strings.ToLower(os.Getenv("ENVIRONMENT"))
	if environment == "" {
		environment = "development"
	}
	isProduction := environment == "production"

	// Set log level
	logLevel := os.Getenv("LOG_LEVEL")
	var level zerolog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn", "warning":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	default:
		if isProduction {
			level = zerolog.InfoLevel
		} else {
			level = zerolog.DebugLevel
		}
	}

	zerolog.SetGlobalLevel(level)

	// Configure output format
	if isProduction {
		// Production: JSON logging for log aggregation
		Logger = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Str("service", "whiteboard-backend").
			Logger()
	} else {
		// Development: Pretty console logging
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		Logger = zerolog.New(output).
			With().
			Timestamp().
			Logger()
	}

	log.Logger = Logger
}

// Debug logs a debug message
func Debug(msg string) *zerolog.Event {
	return Logger.Debug().Str("msg", msg)
}

// Info logs an info message
func Info(msg string) *zerolog.Event {
	return Logger.Info().Str("msg", msg)
}

// Warn logs a warning message
func Warn(msg string) *zerolog.Event {
	return Logger.Warn().Str("msg", msg)
}

// Error logs an error message
func Error(msg string) *zerolog.Event {
	return Logger.Error().Str("msg", msg)
}

// Fatal logs a fatal message and exits
func Fatal(msg string) *zerolog.Event {
	return Logger.Fatal().Str("msg", msg)
}
