package config

import (
	"os"
)

type Config struct {
	DatabaseURL   string
	CSQAQAPIKey   string
	Environment   string
}

func Load() *Config {
	// Default MySQL connection string
	defaultDSN := "root:Wyj250413.@tcp(23.254.215.62:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"

	return &Config{
		DatabaseURL: getEnv("DATABASE_URL", defaultDSN),
		CSQAQAPIKey: getEnv("CSQAQ_API_KEY", "WPXHV1H7O5Y8N8W6R8U1N249"),
		Environment: getEnv("ENVIRONMENT", "production"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}