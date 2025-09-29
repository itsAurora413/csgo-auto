package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Initialize(databaseURL string) (*gorm.DB, error) {
	// Parse MySQL DSN or use the provided URL directly
	var dsn string
	if databaseURL == "" || databaseURL == "csgo_trader.db" {
		// Default MySQL connection
		dsn = "root:Wyj250413.@tcp(23.254.215.62:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
	} else {
		dsn = databaseURL
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
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

func GetDB(db *gorm.DB) *gorm.DB {
	return db
}
