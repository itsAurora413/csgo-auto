package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// API响应结构体
type GoodResponse struct {
	Code int64 `json:"code"`
	Data struct {
		GoodsInfo struct {
			ID             int64   `json:"id"`
			MarketHashName string  `json:"market_hash_name"`
			Name           string  `json:"name"`
			YyypSellPrice  float64 `json:"yyyp_sell_price"`
			BuffSellPrice  float64 `json:"buff_sell_price"`
		} `json:"goods_info"`
	} `json:"data"`
}

// 商品信息结构体
type GoodInfo struct {
	ID             int64
	MarketHashName string
	Name           string
	YyypSellPrice  float64
	BuffSellPrice  float64
}

const (
	apiToken    = "UAXMU177X578K1Q9E1G0N5M8"
	apiBaseURL  = "https://api.csqaq.com/api/v1"
	databaseDSN = "root:Wyj250413.@tcp(192.3.81.194:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local"
	minPrice    = 3.0
	maxPrice    = 50.0
)

var (
	client = &http.Client{Timeout: 15 * time.Second}
	db     *sql.DB
)

// 绑定IP
func bindIP() {
	log.Printf("🔗 绑定IP...")

	req, _ := http.NewRequest("POST", apiBaseURL+"/sys/bind_local_ip", nil)
	req.Header.Set("ApiToken", apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ 绑定IP失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		log.Printf("⚠️ IP绑定被限流（最近已绑定）")
		return
	}

	if resp.StatusCode == 200 {
		log.Printf("✅ IP绑定成功")
	} else {
		log.Printf("❌ IP绑定失败，状态码: %d", resp.StatusCode)
	}
}

// 获取商品信息（带重试）
func fetchGood(id int) *GoodInfo {
	for retry := 1; retry <= 3; retry++ {
		url := fmt.Sprintf("%s/info/good?id=%d", apiBaseURL, id)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("ApiToken", apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if retry < 3 {
				log.Printf("⚠️ 请求失败 good_id %d (重试 %d/3): %v", id, retry, err)
				time.Sleep(1 * time.Second) // 重试前等待1秒
				continue
			}
			log.Printf("❌ 请求失败 good_id %d: %v", id, err)
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == 429 {
			if retry < 3 {
				log.Printf("⚠️ 限流 good_id %d (重试 %d/3)", id, retry)
				time.Sleep(1 * time.Second) // 重试前等待1秒
				continue
			}
			log.Printf("❌ 限流 good_id %d (已重试3次)", id)
			return nil
		}

		if resp.StatusCode != 200 {
			if retry < 3 {
				log.Printf("⚠️ HTTP错误 good_id %d: %d (重试 %d/3)", id, resp.StatusCode, retry)
				time.Sleep(1 * time.Second) // 重试前等待1秒
				continue
			}
			log.Printf("❌ HTTP错误 good_id %d: %d", id, resp.StatusCode)
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if retry < 3 {
				log.Printf("⚠️ 读取响应失败 good_id %d (重试 %d/3): %v", id, retry, err)
				time.Sleep(1 * time.Second) // 重试前等待1秒
				continue
			}
			log.Printf("❌ 读取响应失败 good_id %d: %v", id, err)
			return nil
		}

		var goodResp GoodResponse
		if err := json.Unmarshal(body, &goodResp); err != nil {
			if retry < 3 {
				log.Printf("⚠️ JSON解析失败 good_id %d (重试 %d/3): %v", id, retry, err)
				time.Sleep(1 * time.Second) // 重试前等待1秒
				continue
			}
			log.Printf("❌ JSON解析失败 good_id %d: %v", id, err)
			return nil
		}

		if goodResp.Code != 200 || goodResp.Data.GoodsInfo.ID == 0 {
			return nil // 商品不存在，不需要重试
		}

		// 成功获取数据
		return &GoodInfo{
			ID:             goodResp.Data.GoodsInfo.ID,
			MarketHashName: goodResp.Data.GoodsInfo.MarketHashName,
			Name:           goodResp.Data.GoodsInfo.Name,
			YyypSellPrice:  goodResp.Data.GoodsInfo.YyypSellPrice,
			BuffSellPrice:  goodResp.Data.GoodsInfo.BuffSellPrice,
		}
	}

	return nil
}

// 保存商品到数据库
func saveGood(good *GoodInfo) {
	// 插入商品信息
	_, err := db.Exec(`
		INSERT INTO csqaq_goods (good_id, market_hash_name, name, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			market_hash_name = VALUES(market_hash_name),
			name = VALUES(name),
			updated_at = CURRENT_TIMESTAMP
	`, good.ID, good.MarketHashName, good.Name)

	if err != nil {
		log.Printf("❌ 保存商品失败 %d: %v", good.ID, err)
		return
	}

	// 如果有价格信息，插入快照
	if good.YyypSellPrice > 0 || good.BuffSellPrice > 0 {
		_, err := db.Exec(`
			INSERT INTO csqaq_good_snapshots (good_id, yyyp_sell_price, buff_sell_price, created_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		`, good.ID, good.YyypSellPrice, good.BuffSellPrice)

		if err != nil {
			log.Printf("❌ 保存快照失败 %d: %v", good.ID, err)
		}
	}
}

// 检查价格范围
func isInPriceRange(good *GoodInfo) bool {
	return (good.YyypSellPrice >= minPrice && good.YyypSellPrice <= maxPrice) ||
		(good.BuffSellPrice >= minPrice && good.BuffSellPrice <= maxPrice)
}

// 检查是否需要过滤的商品类型
func shouldFilterOut(name string) bool {
	lowerName := strings.ToLower(name)

	// 检查挂件和纪念品
	hasGuajian := strings.Contains(name, "挂件")
	hasJinianpin := strings.Contains(name, "纪念品")

	if strings.Contains(name, "★") || // 刀具（带星标）
		strings.Contains(name, "手套") ||
		strings.Contains(name, "贴纸") ||
		strings.Contains(name, "印花") ||
		strings.Contains(name, "胶囊") ||
		strings.Contains(name, "探员") ||
		strings.Contains(name, "音乐盒") ||
		strings.Contains(name, "钥匙") ||
		strings.Contains(name, "通行证") ||
		strings.Contains(name, "涂鸦") ||
		strings.Contains(name, "收藏品") ||
		strings.Contains(name, "武器箱") ||
		// 额外中文过滤
		strings.Contains(name, "布章") ||
		strings.Contains(name, "特工") ||
		strings.Contains(name, "徽章") ||
		strings.Contains(name, "挂饰") ||
		strings.Contains(name, "缀饰") ||
		strings.Contains(name, "徽记") ||
		strings.Contains(name, "补丁") ||
		strings.Contains(name, "人偶") ||
		strings.Contains(name, "人形") ||
		strings.Contains(name, "代理人") ||
		strings.Contains(name, "人质") ||
		strings.Contains(name, "徽章包") ||
		strings.Contains(name, "补章") ||
		strings.Contains(name, "德拉戈米尔 | 军刀勇士") ||
		strings.Contains(name, "纪念包") ||
		// 英文小写匹配
		strings.Contains(lowerName, "sticker") ||
		strings.Contains(lowerName, "patch") ||
		strings.Contains(lowerName, "agent") ||
		strings.Contains(lowerName, "music kit") ||
		strings.Contains(lowerName, "souvenir") ||
		strings.Contains(lowerName, "case") ||
		strings.Contains(lowerName, "capsule") ||
		strings.Contains(lowerName, "graffiti") ||
		strings.Contains(lowerName, "key") ||
		strings.Contains(lowerName, "pass") ||
		hasGuajian ||
		hasJinianpin {
		return true
	}

	return false
}

// 初始化数据库
func initDB() {
	var err error
	db, err = sql.Open("mysql", databaseDSN)
	if err != nil {
		log.Fatalf("❌ 数据库连接失败: %v", err)
	}

	// 创建表
	createTables := []string{
		`CREATE TABLE IF NOT EXISTS csqaq_goods (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT UNIQUE NOT NULL,
			market_hash_name TEXT,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS csqaq_good_snapshots (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL,
			yyyp_sell_price DOUBLE,
			buff_sell_price DOUBLE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	}

	for _, sql := range createTables {
		if _, err := db.Exec(sql); err != nil {
			log.Fatalf("❌ 创建表失败: %v", err)
		}
	}

	log.Printf("✅ 数据库初始化完成")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 初始化数据库
	initDB()
	defer db.Close()

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("🛑 收到停止信号，正在退出...")
		cancel()
	}()

	// 启动IP绑定定时器
	go func() {
		bindIP() // 立即绑定一次
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				bindIP()
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Printf("🚀 开始处理商品，范围: 0 - 24041")
	log.Printf("💡 逻辑: 每1秒查询一个商品，每30秒重新绑定IP")

	successCount := 0
	errorCount := 0
	skippedCount := 0
	priceRangeCount := 0
	startTime := time.Now()

	// 主处理循环
	for id := 23458; id <= 24041; id++ {
		select {
		case <-ctx.Done():
			log.Printf("🛑 收到停止信号，已处理 %d 个商品", id)
			goto finish
		default:
		}

		// 等待1秒
		log.Printf("⏱️ 等待1秒...")
		time.Sleep(1 * time.Second)

		// 获取商品信息
		good := fetchGood(id)
		if good == nil {
			errorCount++
			continue
		}

		// 检查是否需要过滤的商品类型
		if shouldFilterOut(good.Name) {
			log.Printf("🚫 过滤 good_id %d (%s) - 不需要的商品类型", good.ID, good.Name)
			skippedCount++
			continue
		}

		// 检查价格范围
		if good.YyypSellPrice > 0 || good.BuffSellPrice > 0 {
			if !isInPriceRange(good) {
				log.Printf("⏭️ 跳过 good_id %d (%s), 价格超出范围: YYYP=%.2f, Buff=%.2f",
					good.ID, good.Name, good.YyypSellPrice, good.BuffSellPrice)
				skippedCount++
				continue
			}
			priceRangeCount++
		}

		// 保存商品
		saveGood(good)
		successCount++

		if isInPriceRange(good) {
			log.Printf("✅ 保存 good_id %d (%s), 价格: YYYP=%.2f, Buff=%.2f",
				good.ID, good.Name, good.YyypSellPrice, good.BuffSellPrice)
		} else {
			log.Printf("✅ 保存 good_id %d (%s), 无价格数据", good.ID, good.Name)
		}

		// 每100个显示进度
		if id%100 == 0 && id > 0 {
			elapsed := time.Since(startTime)
			rate := float64(id) / elapsed.Seconds()
			progress := float64(id) / 24042.0 * 100

			log.Printf("📊 进度: %.1f%% (%d/24042), 成功: %d, 错误: %d, 价格范围内: %d, 跳过: %d, 速率: %.2f/s",
				progress, id, successCount, errorCount, priceRangeCount, skippedCount, rate)
		}
	}

finish:
	elapsed := time.Since(startTime)
	log.Printf("\n🎉 处理完成！")
	log.Printf("📊 总计: 成功 %d, 错误 %d, 价格范围内 %d, 跳过 %d",
		successCount, errorCount, priceRangeCount, skippedCount)
	log.Printf("⏱️ 总用时: %v, 平均速率: %.2f/s",
		elapsed.Truncate(time.Second), float64(successCount)/elapsed.Seconds())
}
