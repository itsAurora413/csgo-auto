package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"csgo-trader/internal/api"
	"csgo-trader/internal/config"
	"csgo-trader/internal/database"
	steamService "csgo-trader/internal/services/steam"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize configuration
	cfg := config.Load()

	// Print YouPin authentication mode
	if cfg.YoupinAppKey != "" && len(cfg.YoupinAppKey) >= 8 {
		log.Printf("🔐 悠悠有品: 开放平台API模式 (AppKey: %s...)", cfg.YoupinAppKey[:8])
	} else {
		log.Println("⚠️  悠悠有品: 开放平台API未配置")
	}

	// Initialize database
	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Initialize services - only Steam service for authentication
	steamSvc := steamService.NewSteamService(cfg.SteamAPIKey)

	// Initialize Gin router
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

    // Serve static files from the build directory
    r.Static("/static", "./web/build/static")
	r.StaticFile("/favicon.ico", "./web/build/favicon.ico")
	r.StaticFile("/manifest.json", "./web/build/manifest.json")
	r.StaticFile("/logo192.png", "./web/build/logo192.png")
    r.GET("/", func(c *gin.Context) {
        c.File("./web/build/index.html")
    })
    // Health check
    r.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "ok"})
    })
	// SPA fallback for client-side routing
	r.NoRoute(func(c *gin.Context) {
		// Preserve API and WS 404s
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/ws" || strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Status(http.StatusNotFound)
			return
		}
		c.File("./web/build/index.html")
	})

	// API routes
    apiGroup := r.Group("/api/v1")
    api.SetupRoutes(apiGroup, db, steamSvc)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
