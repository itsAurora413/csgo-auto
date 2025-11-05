package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"csqaq-sampler/internal/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// Define flags
	var (
		dsn             = flag.String("dsn", "", "数据库DSN (格式: user:password@tcp(host:port)/dbname?charset=utf8mb4)")
		dryRun          = flag.Bool("dry-run", false, "仅显示将要执行的操作，不真正修改数据库")
		showOnlyMissing = flag.Bool("show-missing", false, "仅显示缺失yyyp_template_id的商品")
	)
	flag.Parse()

	// Load .env if exists
	_ = godotenv.Load()

	// 如果命令行没提供DSN，从环境变量读取
	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
		if *dsn == "" {
			log.Fatal("❌ 错误: 必须提供 --dsn 参数或设置 DATABASE_URL 环境变量\n" +
				"用法: go run cmd/migrate-yyyp-template-id/main.go --dsn \"root:password@tcp(host:port)/dbname?charset=utf8mb4\"\n" +
				"或者: export DATABASE_URL=\"root:password@tcp(host:port)/dbname?charset=utf8mb4\"\n" +
				"     go run cmd/migrate-yyyp-template-id/main.go")
		}
	}

	fmt.Println(stringRepeat("=", 80))
	fmt.Println("🔄 悠悠有品模板ID迁移工具")
	fmt.Println(stringRepeat("=", 80))

	// Connect to database
	fmt.Printf("\n📡 连接数据库: %s\n", maskDSN(*dsn))
	db, err := gorm.Open(mysql.Open(*dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 数据库连接失败: %v\n", err)
	}

	// Check for foreign key constraint issues
	fmt.Println("\n📝 检查数据库结构和外键约束...")

	// Step 1: Drop foreign key if it exists
	if err := dropForeignKeyIfExists(db); err != nil {
		log.Printf("⚠️  删除外键时出现问题: %v (继续)\n", err)
	}

	// Step 2: Auto migrate to add column if not exists
	if err := db.AutoMigrate(&models.CSQAQGood{}); err != nil {
		log.Fatalf("❌ 数据库迁移失败: %v\n", err)
	}

	// Step 3: Recreate foreign key
	if err := recreateForeignKey(db); err != nil {
		log.Printf("⚠️  重新创建外键时出现问题: %v (继续)\n", err)
	}

	fmt.Println("✅ 数据库结构已更新")

	// Check if column exists
	if !db.Migrator().HasColumn(&models.CSQAQGood{}, "yyyp_template_id") {
		fmt.Println("❌ 错误: yyyp_template_id 列不存在")
		os.Exit(1)
	}

	// Get statistics
	fmt.Println("\n📊 统计信息:")

	var totalGoods int64
	if err := db.Model(&models.CSQAQGood{}).Count(&totalGoods).Error; err != nil {
		log.Fatalf("❌ 查询失败: %v\n", err)
	}
	fmt.Printf("   总商品数: %d\n", totalGoods)

	var withTemplateID int64
	if err := db.Model(&models.CSQAQGood{}).Where("yyyp_template_id IS NOT NULL").Count(&withTemplateID).Error; err != nil {
		log.Fatalf("❌ 查询失败: %v\n", err)
	}
	fmt.Printf("   已有yyyp_template_id: %d\n", withTemplateID)

	var needsUpdate int64
	if err := db.Model(&models.CSQAQGood{}).Where("yyyp_template_id IS NULL").Count(&needsUpdate).Error; err != nil {
		log.Fatalf("❌ 查询失败: %v\n", err)
	}
	fmt.Printf("   需要更新: %d\n", needsUpdate)

	// Show only missing flag
	if *showOnlyMissing {
		fmt.Println("\n📋 缺失yyyp_template_id的商品列表:")
		var goods []models.CSQAQGood
		if err := db.Where("yyyp_template_id IS NULL").Limit(20).Find(&goods).Error; err != nil {
			log.Fatalf("❌ 查询失败: %v\n", err)
		}
		for _, g := range goods {
			fmt.Printf("   GoodID: %d, Name: %s\n", g.GoodID, g.Name)
		}
		return
	}

	if needsUpdate == 0 {
		fmt.Println("\n✅ 所有商品都已有yyyp_template_id，无需更新")
		return
	}

	// Dry run mode
	if *dryRun {
		fmt.Println("\n🔍 [DRY RUN] 将执行以下操作:")
		fmt.Printf("   从 csqaq_good_snapshots 获取 good_id 对应的 yyyp_template_id\n")
		fmt.Printf("   更新 %d 个商品的 yyyp_template_id\n", needsUpdate)
		fmt.Println("\n💡 提示: 去掉 --dry-run 标志来真正执行迁移")
		return
	}

	// Real migration
	fmt.Printf("\n🔄 开始迁移 %d 个商品...\n", needsUpdate)

	// Get goods that need update
	var goodsToUpdate []models.CSQAQGood
	if err := db.Where("yyyp_template_id IS NULL").Find(&goodsToUpdate).Error; err != nil {
		log.Fatalf("❌ 查询失败: %v\n", err)
	}

	successCount := 0
	failureCount := 0

	for idx, good := range goodsToUpdate {
		// Get the latest snapshot with yyyp_template_id for this good
		var snapshot models.CSQAQGoodSnapshot
		if err := db.Where("good_id = ? AND yyyp_template_id IS NOT NULL", good.GoodID).
			Order("created_at DESC").
			First(&snapshot).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// 没有找到有效的snapshot
				failureCount++
				if (idx + 1) % 100 == 0 {
					fmt.Printf("   [%d/%d] GoodID %d: 没有有效的yyyp_template_id\n",
						idx+1, len(goodsToUpdate), good.GoodID)
				}
				continue
			}
			failureCount++
			log.Printf("❌ 查询GoodID %d的snapshot失败: %v\n", good.GoodID, err)
			continue
		}

		// Update the good with yyyp_template_id
		if err := db.Model(&good).Update("yyyp_template_id", snapshot.YYYPTemplateID).Error; err != nil {
			failureCount++
			log.Printf("❌ 更新GoodID %d失败: %v\n", good.GoodID, err)
			continue
		}

		successCount++

		// Show progress every 100 items
		if (idx + 1) % 100 == 0 {
			fmt.Printf("   [%d/%d] 已处理 %d 个，成功 %d 个\n",
				idx+1, len(goodsToUpdate), idx+1, successCount)
		}
	}

	fmt.Println("\n" + stringRepeat("=", 80))
	fmt.Println("✅ 迁移完成")
	fmt.Println(stringRepeat("=", 80))
	fmt.Printf("总处理: %d\n", len(goodsToUpdate))
	fmt.Printf("成功更新: %d\n", successCount)
	fmt.Printf("失败: %d (无有效yyyp_template_id)\n", failureCount)
	fmt.Printf("成功率: %.1f%%\n", float64(successCount)/float64(len(goodsToUpdate))*100)

	// Final statistics
	if err := db.Model(&models.CSQAQGood{}).Where("yyyp_template_id IS NOT NULL").Count(&withTemplateID).Error; err == nil {
		fmt.Printf("\n最终统计:\n")
		fmt.Printf("   已有yyyp_template_id: %d\n", withTemplateID)
		fmt.Printf("   缺失yyyp_template_id: %d\n", totalGoods-withTemplateID)
	}
}

// Utility functions
func maskDSN(dsn string) string {
	// Simple mask: show only host and database
	if len(dsn) > 50 {
		return dsn[:20] + "****" + dsn[len(dsn)-20:]
	}
	return "****"
}

// String repeat helper (since Go doesn't have built-in)
func stringRepeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

// dropForeignKeyIfExists drops the foreign key constraint if it exists
func dropForeignKeyIfExists(db *gorm.DB) error {
	// Directly query and drop all foreign keys on csqaq_good_snapshots referencing good_id
	// We use raw query results as strings instead of structured scanning

	rows, err := db.Raw(`
		SELECT CONSTRAINT_NAME
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_NAME = 'csqaq_good_snapshots'
		AND COLUMN_NAME = 'good_id'
		AND REFERENCED_TABLE_NAME = 'csqaq_goods'
	`).Rows()

	if err != nil {
		log.Printf("    ℹ️  查询外键信息: %v\n", err)
		return nil // Continue anyway
	}
	defer rows.Close()

	// Drop each foreign key found
	for rows.Next() {
		var constraintName string
		if err := rows.Scan(&constraintName); err != nil {
			log.Printf("    ℹ️  扫描外键名称失败: %v\n", err)
			continue
		}

		if constraintName == "" {
			continue
		}

		dropSQL := fmt.Sprintf("ALTER TABLE csqaq_good_snapshots DROP FOREIGN KEY `%s`", constraintName)
		if err := db.Exec(dropSQL).Error; err != nil {
			log.Printf("    ℹ️  无法删除外键 %s: %v\n", constraintName, err)
		} else {
			log.Printf("    ✓ 删除外键: %s\n", constraintName)
		}
	}

	return nil
}

// recreateForeignKey recreates the foreign key constraint
func recreateForeignKey(db *gorm.DB) error {
	// Ensure both columns are BIGINT NOT NULL
	if err := db.Exec(`
		ALTER TABLE csqaq_goods
		MODIFY COLUMN good_id BIGINT NOT NULL
	`).Error; err != nil {
		log.Printf("    ℹ️  修改 csqaq_goods.good_id: %v\n", err)
	}

	if err := db.Exec(`
		ALTER TABLE csqaq_good_snapshots
		MODIFY COLUMN good_id BIGINT NOT NULL
	`).Error; err != nil {
		log.Printf("    ℹ️  修改 csqaq_good_snapshots.good_id: %v\n", err)
	}

	// Recreate foreign key
	if err := db.Exec(`
		ALTER TABLE csqaq_good_snapshots
		ADD CONSTRAINT csqaq_good_snapshots_FK_good_id
		FOREIGN KEY (good_id)
		REFERENCES csqaq_goods(good_id)
		ON DELETE CASCADE
		ON UPDATE CASCADE
	`).Error; err != nil {
		// FK might already exist, which is fine
		if !contains(err.Error(), "already exists") {
			log.Printf("    ⚠️  重新创建外键失败: %v\n", err)
		} else {
			log.Printf("    ℹ️  外键已存在\n")
		}
	} else {
		log.Printf("    ✓ 重新创建外键成功\n")
	}

	return nil
}

// contains checks if a string contains a substring
func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
