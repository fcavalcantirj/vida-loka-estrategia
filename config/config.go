package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all configuration for the application
type Config struct {
	// WhatsApp configuration
	WhatsApp WhatsAppConfig `json:"whatsapp"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Game configuration
	Game GameConfig `json:"game"`

	// Server configuration
	Server ServerConfig `json:"server"`
}

// WhatsAppConfig holds WhatsApp specific configuration
type WhatsAppConfig struct {
	// Path to store WhatsApp session data
	StoreDir string `json:"store_dir"`

	// Client device name
	ClientName string `json:"client_name"`

	// Auto-reply timeout in seconds
	AutoReplyTimeout int `json:"auto_reply_timeout"`
}

// DatabaseConfig holds database specific configuration
type DatabaseConfig struct {
	// Database driver (sqlite3)
	Driver string `json:"driver"`

	// Database connection string
	DSN string `json:"dsn"`
}

// GameConfig holds game specific configuration
type GameConfig struct {
	// Default starting XP
	DefaultXP int `json:"default_xp"`

	// Default starting money
	DefaultMoney int `json:"default_money"`

	// Default starting influence
	DefaultInfluence int `json:"default_influence"`

	// Default starting stress
	DefaultStress int `json:"default_stress"`

	// Default zone
	DefaultZone string `json:"default_zone"`

	// Default sub-zone
	DefaultSubZone string `json:"default_sub_zone"`

	// Time between events in minutes
	EventInterval int `json:"event_interval"`

	// Probability of random events (0-100)
	RandomEventProbability int `json:"random_event_probability"`
}

// ServerConfig holds server specific configuration
type ServerConfig struct {
	// Server port
	Port string `json:"port"`

	// Log level (debug, info, warn, error)
	LogLevel string `json:"log_level"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		WhatsApp: WhatsAppConfig{
			StoreDir:         "./whatsapp-store",
			ClientName:       "VIDA LOKA STRATEGY",
			AutoReplyTimeout: 300,
		},
		Database: DatabaseConfig{
			Driver: "sqlite3",
			DSN:    "./vida-loka.db",
		},
		Game: GameConfig{
			DefaultXP:              0,
			DefaultMoney:           100,
			DefaultInfluence:       0,
			DefaultStress:          0,
			DefaultZone:            "",
			DefaultSubZone:         "",
			EventInterval:          60,
			RandomEventProbability: 20,
		},
		Server: ServerConfig{
			Port:     "8080",
			LogLevel: "info",
		},
	}
}

// LoadConfig loads configuration from a file
func LoadConfig(path string) (Config, error) {
	config := DefaultConfig()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return config, err
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create default config file
		file, err := os.Create(path)
		if err != nil {
			return config, err
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(config); err != nil {
			return config, err
		}

		return config, nil
	}

	// Read config file
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}

	return config, nil
}

// SaveConfig saves configuration to a file
func SaveConfig(config Config, path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create or truncate file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write config to file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return err
	}

	return nil
}
