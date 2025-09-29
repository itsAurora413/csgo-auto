package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"csqaq-sampler/internal/config"
	"csqaq-sampler/internal/services"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize database
	db, err := initializeDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("CSQAQ Standalone Sampler initialized successfully")
	log.Printf("Using API Key: %s", maskAPIKey(cfg.CSQAQAPIKey))
	log.Printf("Database connected: %s", maskDSN(cfg.DatabaseURL))

	// Create CSQAQ API request function
	makeRequest := func(endpoint string, params map[string]string) ([]byte, error) {
		return makeCSQAQRequest(endpoint, params, cfg.CSQAQAPIKey)
	}

	// Start enhanced sampler
	sampler := services.StartEnhancedCSQAQSampler(db, makeRequest)

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Println("CSQAQ Sampler is running. Press Ctrl+C to stop...")
	<-c

	log.Println("Shutting down sampler...")
	sampler.Stop()
	log.Println("Sampler stopped gracefully")
}

func initializeDatabase(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Set connection pool parameters
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database initialized successfully")
	return db, nil
}

// CSQAQ API constants and functions
const CSQAQ_API_BASE = "https://api.csqaq.com/api/v1/"

var lastBindTime time.Time

func ensureIPBound(apiKey string) error {
	// Bind IP every 35 seconds to avoid rate limits
	if time.Since(lastBindTime) < 35*time.Second {
		return nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := CSQAQ_API_BASE + "sys/bind_local_ip"

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("ApiToken", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to bind local IP, status: %d, response: %s", resp.StatusCode, string(body))
	}

	lastBindTime = time.Now()
	fmt.Printf("Successfully bound local IP to CSQAQ API\n")
	return nil
}

func makeCSQAQRequest(endpoint string, params map[string]string, apiKey string) ([]byte, error) {
	// Ensure IP is bound before making API requests
	if err := ensureIPBound(apiKey); err != nil {
		fmt.Printf("Warning: Failed to bind local IP: %v\n", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Build URL with parameters
	reqURL := CSQAQ_API_BASE + endpoint
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Add(k, v)
		}
		reqURL += "?" + values.Encode()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("ApiToken", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// Utility functions
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}

func maskDSN(dsn string) string {
	if len(dsn) <= 20 {
		return "****"
	}
	return dsn[:10] + "****" + dsn[len(dsn)-10:]
}