// Package config provides application configuration loading from environment
// variables, flags, and optional JSON config file.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration values.
type Config struct {
	// Database connection string (DSN).
	DatabaseDSN string

	// SpeechProvider selects speech recognition provider: "mock" or "salute".
	SpeechProvider string

	// LLMProvider selects LLM provider: "mock" or "gigachat".
	LLMProvider string

	// SaluteSpeechAPIKey is the API key for SaluteSpeech (required if SpeechProvider == "salute").
	SaluteSpeechAPIKey string

	// GigaChatAPIKey is the API key for GigaChat (required if LLMProvider == "gigachat").
	GigaChatAPIKey string

	// UserID is the default user ID for CLI mode.
	UserID string

	// migrationsPath is the path to the migrations directory.
	migrationsPath string

	// MongoDBDSN is the connection string for MongoDB GridFS.
	MongoDBDSN string

	// MongoDBDatabase is the name of the MongoDB database.
	MongoDBDatabase string

	// MongoDBBucket is the name of the GridFS bucket.
	MongoDBBucket string
}

// Load reads configuration from environment variables and returns a Config.
// It supports .env file loading via godotenv for local development.
func Load() (*Config, error) {
	// Try to load .env file (non-fatal if not found)
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseDSN:        envOr("DATABASE_DSN", "postgres://loader:1234@localhost:5434/truecode_db?sslmode=disable"),
		SpeechProvider:     envOr("SPEECH_PROVIDER", "mock"),
		LLMProvider:        envOr("LLM_PROVIDER", "mock"),
		SaluteSpeechAPIKey: os.Getenv("SALUTE_SPEECH_KEY"),
		GigaChatAPIKey:     os.Getenv("GIGA_CHAT_KEY"),
		UserID:             os.Getenv("USER_ID"),
	}

	if cfg.UserID == "" {
		cfg.UserID = "1"
	}

	// Validate required fields
	if cfg.SpeechProvider == "salute" && cfg.SaluteSpeechAPIKey == "" {
		return nil, fmt.Errorf("SPEECH_PROVIDER=salute requires SALUTE_SPEECH_KEY")
	}

	if cfg.LLMProvider == "gigachat" && cfg.GigaChatAPIKey == "" {
		return nil, fmt.Errorf("LLM_PROVIDER=gigachat requires GIGA_CHAT_KEY")
	}

	// Set migrations path from env or default
	if cfg.migrationsPath == "" {
		cfg.migrationsPath = "migrations"
	}

	// Set MongoDB config from env or defaults
	cfg.MongoDBDSN = envOr("MONGODB_DSN", "mongodb://admin:1234@127.0.0.1:27017/ContentStore?authSource=admin")
	cfg.MongoDBDatabase = envOr("MONGODB_DATABASE", "ContentStore")
	cfg.MongoDBBucket = envOr("MONGODB_BUCKET", "Content")

	return cfg, nil
}

// MigrationsPath returns the path to the migrations directory.
func (c *Config) MigrationsPath() string {
	return c.migrationsPath
}

// envOr returns the environment variable value for key, or defaultVal if not set.
func envOr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
