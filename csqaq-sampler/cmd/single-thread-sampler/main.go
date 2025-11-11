package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"csqaq-sampler/internal/config"
	"csqaq-sampler/internal/services"
	"csqaq-sampler/internal/services/youpin"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	log.Println("========================================")
	log.Println("单线程采样器 - 1秒间隔，无代理")
	log.Println("========================================")

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize database
	db, err := initializeDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	log.Println("单线程采样器初始化完成")
	log.Printf("数据库: %s\n", maskDSN(cfg.DatabaseURL))

	// Initialize YouPin OpenAPI client (不使用代理)
	ypClient, err := youpin.NewOpenAPIClientWithDefaultKeys()
	if err != nil {
		log.Fatal("创建YouPin OpenAPI客户端失败:", err)
	}
	log.Println("YouPin OpenAPI客户端初始化成功 (无代理)")

	// 使用内置 Token 客户端 (不使用代理)
	deviceToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiI5Y2IyNjJkYWFlMDE0NjdkOWRkOTdkZDY2NGVmZjhmMiIsIm5hbWVpZCI6IjE2NDUyMzEiLCJJZCI6IjE2NDUyMzEiLCJ1bmlxdWVfbmFtZSI6IllQMDAwMTY0NTIzMSIsIk5hbWUiOiJZUDAwMDE2NDUyMzEiLCJ2ZXJzaW9uIjoiSTFqIiwibmJmIjoxNzYyODI3NDg3LCJleHAiOjE3NjM2OTE0ODcsImlzcyI6InlvdXBpbjg5OC5jb20iLCJkZXZpY2VJZCI6ImU3ZGYzOWQ1LTEzZjYtNDZmMS1hNDI0LTFmZDU5YjU4NTk4OCIsImF1ZCI6InVzZXIifQ.CRb9VDDtCVvJBlvzLjqTWxYH_A7hBxt8mBluB00WiRE"
	var tokenClient *youpin.OpenAPIClient
	if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(deviceToken); err == nil {
		tokenClient = c
		log.Println("YouPin Token客户端初始化成功 (无代理)")
	} else {
		log.Printf("Token客户端初始化失败: %v，将使用OpenAPI客户端代替\n", err)
		tokenClient = ypClient
	}

	// Create single-thread sampler
	singleThreadSampler, err := services.NewSingleThreadSampler(db, ypClient, tokenClient)
	if err != nil {
		log.Fatal("创建单线程采样器失败:", err)
	}

	// Start sampler
	singleThreadSampler.Start()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Println("单线程采样器运行中... (按 Ctrl+C 停止)")
	<-c

	log.Println("正在关闭采样器...")
	singleThreadSampler.Stop()
	log.Println("采样器已停止")
}

func initializeDatabase(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("连接MySQL数据库失败: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取underlying sql.DB失败: %w", err)
	}

	// Set connection pool parameters
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("数据库初始化完成")

	return db, nil
}

// Utility functions
func maskDSN(dsn string) string {
	if len(dsn) <= 20 {
		return "****"
	}
	return dsn[:10] + "****" + dsn[len(dsn)-10:]
}
