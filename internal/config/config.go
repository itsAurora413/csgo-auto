package config

import (
	"os"
)

type Config struct {
	DatabaseURL  string
	SteamAPIKey  string
	BuffAPIKey   string
	YoupinAPIKey string // 传统Token认证（抓包方式）
	JWTSecret    string
	Port         string
	Environment  string

	// 悠悠有品开放平台配置
	YoupinOpenAPI    bool   // 是否使用开放平台API
	YoupinAppKey     string // 开放平台AppKey
	YoupinPrivateKey string // 开放平台RSA私钥（Base64编码）
}

func Load() *Config {
	// Default MySQL connection string
	defaultDSN := "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"

	// 判断是否使用开放平台API
	useOpenAPI := getEnv("YOUPIN_USE_OPEN_API", "false") == "true"

	return &Config{
		DatabaseURL:  getEnv("DATABASE_URL", defaultDSN),
		SteamAPIKey:  getEnv("STEAM_API_KEY", ""),
		BuffAPIKey:   getEnv("BUFF_API_KEY", ""),
		YoupinAPIKey: getEnv("YOUPIN_API_KEY", ""),
		JWTSecret:    getEnv("JWT_SECRET", "your-secret-key"),
		Port:         getEnv("PORT", "8080"),
		Environment:  getEnv("ENVIRONMENT", "development"),

		// 开放平台配置
		YoupinOpenAPI:    useOpenAPI,
		YoupinAppKey:     getEnv("YOUPIN_APP_KEY", ""),
		YoupinPrivateKey: getEnv("YOUPIN_PRIVATE_KEY", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
