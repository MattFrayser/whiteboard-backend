package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment
type Config struct {
	// Environment settings
	Environment    string
	IsProduction   bool
	AllowedDomains []string

	// TLS settings
	TLSCertFile string
	TLSKeyFile  string
	UseTLS      bool
}

// Load reads configuration from environment variables
func Load() *Config {
	// Load .env file if present
	godotenv.Load()

	// Parse environment
	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	// Parse allowed domains
	allowedDomains := strings.Split(os.Getenv("DOMAINS"), ",")

	// Parse TLS settings
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	useTLS := certFile != "" && keyFile != ""

	return &Config{
		Environment:    environment,
		IsProduction:   environment == "production",
		AllowedDomains: allowedDomains,

		TLSCertFile: certFile,
		TLSKeyFile:  keyFile,
		UseTLS:      useTLS,
	}
}
