package config

import "time"

// Config holds the application configuration.
type Config struct {
	Server      ServerConfig
	Auth        AuthConfig
	Persistence PersistenceConfig
	Locales     []string
	Features    FeatureFlags
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Host string
	Port string
}

// AuthConfig configures authentication.
type AuthConfig struct {
	SessionKey     string
	SessionTimeout time.Duration
}

// PersistenceConfig configures database connection.
type PersistenceConfig struct {
	Driver string
	DSN    string
}

// FeatureFlags enables/disables features.
type FeatureFlags struct {
	EnableWebSocket bool
	EnableDigests   bool
	EnableRetries   bool
}

// Defaults returns default configuration.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: "8481",
		},
		Auth: AuthConfig{
			SessionKey:     "demo-secret-key-change-in-production",
			SessionTimeout: time.Hour,
		},
		Persistence: PersistenceConfig{
			Driver: "sqlite",
			DSN:    ":memory:",
		},
		Locales: []string{"en", "es"},
		Features: FeatureFlags{
			EnableWebSocket: true,
			EnableDigests:   false,
			EnableRetries:   true,
		},
	}
}
