package config

import (
	"os"
)

type Config struct {
	DatabaseURL string
	CSQAQAPIKey string
	Environment string

	// Dual account configuration
	YoupinAccount1 *YoupinAccount
	YoupinAccount2 *YoupinAccount
}

type YoupinAccount struct {
	APIKey      string
	PrivateKey  string
	Token       string
	AccountName string
}

func Load() *Config {
	// Default MySQL connection string
	defaultDSN := "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"

	return &Config{
		DatabaseURL: getEnv("DATABASE_URL", defaultDSN),
		CSQAQAPIKey: getEnv("CSQAQ_API_KEY", "WPXHV1H7O5Y8N8W6R8U1N249"),
		Environment: getEnv("ENVIRONMENT", "production"),

		// Initialize dual accounts (optional)
		YoupinAccount1: &YoupinAccount{
			APIKey:      getEnv("YOUPIN_API_KEY_1", "1645231"),
			PrivateKey:  getEnv("YOUPIN_PRIVATE_KEY_1", ""),
			Token:       getEnv("YOUPIN_TOKEN_1", ""),
			AccountName: "AccountA",
		},
		YoupinAccount2: &YoupinAccount{
			APIKey:      getEnv("YOUPIN_API_KEY_2", "12919014"),
			PrivateKey:  getEnv("YOUPIN_PRIVATE_KEY_2", ""),
			Token:       getEnv("YOUPIN_TOKEN_2", ""),
			AccountName: "AccountB",
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
