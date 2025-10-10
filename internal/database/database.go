package database

import (
    "fmt"
    "log"
    "time"

    "csgo-trader/internal/models"
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

	// Migrations: ensure new columns exist
	if err := ensureCSQAQSnapshotTemplateID(db); err != nil {
		log.Printf("Migration warning: %v", err)
	}
	return db, nil
}

func GetDB(db *gorm.DB) *gorm.DB {
    return db
}

// ensureCSQAQSnapshotTemplateID adds yyyp_template_id column to csqaq_good_snapshots if missing
func ensureCSQAQSnapshotTemplateID(db *gorm.DB) error {
    // Prefer GORM migrator checks
    if db.Migrator().HasColumn(&models.CSQAQGoodSnapshot{}, "yyyp_template_id") {
        return nil
    }

    // Try adding via GORM migrator
    if err := db.Migrator().AddColumn(&models.CSQAQGoodSnapshot{}, "YYYPTemplateID"); err == nil {
        // Ensure index as well
        _ = db.Migrator().CreateIndex(&models.CSQAQGoodSnapshot{}, "YYYPTemplateID")
        log.Println("Added column yyyp_template_id via GORM migrator")
        return nil
    }

    // Fallback to raw SQL (in case migrator fails)
    var count int64
    checkSQL := `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'csqaq_good_snapshots' AND column_name = 'yyyp_template_id'`
    if err := db.Raw(checkSQL).Scan(&count).Error; err != nil {
        return fmt.Errorf("failed checking yyyp_template_id column: %w", err)
    }
    if count > 0 {
        return nil
    }
    alterSQL := `ALTER TABLE csqaq_good_snapshots ADD COLUMN yyyp_template_id BIGINT NULL`
    if err := db.Exec(alterSQL).Error; err != nil {
        return fmt.Errorf("failed adding yyyp_template_id column: %w", err)
    }
    _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cgs_yyyp_template_id ON csqaq_good_snapshots (yyyp_template_id)`).Error
    log.Println("Added column yyyp_template_id to csqaq_good_snapshots")
    return nil
}
