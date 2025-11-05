package main

import (
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"csqaq-sampler/internal/models"
)

func main() {
	// 连接MySQL
	dsn := "root:Wyj250413.@tcp(23.254.215.66:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	fmt.Println("数据库连接成功")
	fmt.Println("=========================================")

	// 查询 good_id = 24024 的数据
	var good models.CSQAQGood
	if err := db.First(&good, "good_id = ?", 24024).Error; err != nil {
		log.Fatalf("查询商品失败: %v", err)
	}

	fmt.Printf("商品ID: %d\n", good.GoodID)
	if good.YYYPTemplateID != nil {
		fmt.Printf("YouPin TemplateID: %d\n", *good.YYYPTemplateID)
	} else {
		fmt.Println("YouPin TemplateID: NULL")
	}
	fmt.Println()

	// 查询该商品的最近5条快照
	var snapshots []models.CSQAQGoodSnapshot
	if err := db.Where("good_id = ?", 24024).Order("created_at DESC").Limit(5).Find(&snapshots).Error; err != nil {
		log.Fatalf("查询快照失败: %v", err)
	}

	fmt.Printf("最近快照 (%d 条):\n", len(snapshots))
	fmt.Println("=========================================")

	for i, snap := range snapshots {
		fmt.Printf("\n[快照 %d]\n", i+1)
		if snap.YYYPTemplateID != nil {
			fmt.Printf("  TemplateID: %d\n", *snap.YYYPTemplateID)
		} else {
			fmt.Println("  TemplateID: NULL")
		}
		if snap.YYYPSellPrice != nil {
			fmt.Printf("  售价: %.2f\n", *snap.YYYPSellPrice)
		}
		if snap.YYYPBuyPrice != nil {
			fmt.Printf("  求购价: %.2f\n", *snap.YYYPBuyPrice)
		}
		fmt.Printf("  创建时间: %s\n", snap.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	fmt.Println()
	fmt.Println("=========================================")
	fmt.Println("发现问题：最新快照的售价是 5.65，但API查询返回的是 16.00")
	fmt.Println("这说明可能是两个不同的数据来源：")
	fmt.Println("  - 快照中的 5.65 来自缓存更新")
	fmt.Println("  - API返回的 16.00 是当前最新值")
}
