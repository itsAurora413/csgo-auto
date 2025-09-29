package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	// Uncomment these imports when main function is enabled
	// "database/sql"
	// "os"
	// "strconv"
	// _ "github.com/go-sql-driver/mysql"
)

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

/*
func main() {
	interval := 3000 // 默认3秒间隔
	if len(os.Args) > 1 {
		interval, _ = strconv.Atoi(os.Args[1])
	}

	fmt.Printf("开始获取饰品数据: ID 12021 到 24041, 间隔 %d 毫秒\n", interval)

	// 打开数据库连接
	db, err := sql.Open("mysql", "root:Wyj250413.@tcp(23.254.215.62:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		fmt.Printf("数据库连接失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 确保表存在
	createTables := []string{
		`CREATE TABLE IF NOT EXISTS csqaq_goods (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT UNIQUE NOT NULL,
			market_hash_name TEXT,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS csqaq_good_snapshots (
			id INT AUTO_INCREMENT PRIMARY KEY,
			good_id BIGINT NOT NULL,
			yyyp_sell_price DOUBLE,
			buff_sell_price DOUBLE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, sql := range createTables {
		_, err = db.Exec(sql)
		if err != nil {
			fmt.Printf("创建表失败: %v\n", err)
			os.Exit(1)
		}
	}

	// API令牌配置 - 只使用令牌2
	apiToken := "WPXHV1H7O5Y8N8W6R8U1N249" // 只使用新令牌

	// 使用单个令牌处理指定范围
	client := &http.Client{Timeout: 15 * time.Second}
	successCount := 0
	errorCount := 0
	priceRangeCount := 0

	fmt.Printf("\n=== 使用令牌: %s ===\n", apiToken)

	fmt.Printf("使用本机地址直接请求API...\n")

	// 固定范围: 12021-24041
	rangeStart := 12021
	rangeEnd := 24041

	fmt.Printf("处理范围: %d - %d (%d 个商品)\n", rangeStart, rangeEnd, rangeEnd-rangeStart+1)

	for id := rangeStart; id <= rangeEnd; id++ {
			good, err := fetchGood(client, apiToken, id)
			if err != nil {
				fmt.Printf("获取 good_id %d 失败: %v\n", id, err)
				errorCount++
				time.Sleep(time.Duration(interval) * time.Millisecond)
				continue
			}

			if good == nil {
				// 商品不存在，跳过
				time.Sleep(time.Duration(interval) * time.Millisecond)
				continue
			}

			goodInfo := good.Data.GoodsInfo

			// 检查价格范围
			inPriceRange := (goodInfo.YyypSellPrice >= 50 && goodInfo.YyypSellPrice <= 300) ||
							(goodInfo.BuffSellPrice >= 50 && goodInfo.BuffSellPrice <= 300)

			if !inPriceRange && (goodInfo.YyypSellPrice > 0 || goodInfo.BuffSellPrice > 0) {
				fmt.Printf("跳过 good_id %d (%s), 价格超出范围: YYYP=%.2f, Buff=%.2f\n",
					goodInfo.ID, goodInfo.Name, goodInfo.YyypSellPrice, goodInfo.BuffSellPrice)
				time.Sleep(time.Duration(interval) * time.Millisecond)
				continue
			}

			// 保存商品信息到数据库
			_, err = db.Exec(`
				INSERT INTO csqaq_goods (good_id, market_hash_name, name, updated_at)
				VALUES (?, ?, ?, CURRENT_TIMESTAMP)
				ON DUPLICATE KEY UPDATE
					market_hash_name = VALUES(market_hash_name),
					name = VALUES(name),
					updated_at = CURRENT_TIMESTAMP
			`, goodInfo.ID, goodInfo.MarketHashName, goodInfo.Name)

			// 如果有价格信息，也保存到快照表
			if goodInfo.YyypSellPrice > 0 || goodInfo.BuffSellPrice > 0 {
				_, err2 := db.Exec(`
					INSERT INTO csqaq_good_snapshots (good_id, yyyp_sell_price, buff_sell_price, created_at)
					VALUES (?, ?, ?, CURRENT_TIMESTAMP)
				`, goodInfo.ID, goodInfo.YyypSellPrice, goodInfo.BuffSellPrice)
				if err2 != nil {
					fmt.Printf("保存快照 good_id %d 失败: %v\n", id, err2)
				}
			}

			if err != nil {
				fmt.Printf("保存 good_id %d 失败: %v\n", id, err)
				errorCount++
			} else {
				if inPriceRange {
					fmt.Printf("✓ 保存 good_id %d (%s), 价格: YYYP=%.2f, Buff=%.2f\n",
						goodInfo.ID, goodInfo.Name, goodInfo.YyypSellPrice, goodInfo.BuffSellPrice)
					priceRangeCount++
				} else {
					fmt.Printf("✓ 保存 good_id %d (%s), 无价格数据\n", goodInfo.ID, goodInfo.Name)
				}
				successCount++
			}

			// 每100个显示进度
			if (id-rangeStart+1)%100 == 0 {
				progress := float64(id-rangeStart+1) / float64(rangeEnd-rangeStart+1) * 100
				fmt.Printf("进度: %.1f%% (%d/%d), 成功: %d, 错误: %d, 价格范围内: %d\n",
					progress, id-rangeStart+1, rangeEnd-rangeStart+1, successCount, errorCount, priceRangeCount)
			}

			time.Sleep(time.Duration(interval) * time.Millisecond)
	}

	fmt.Printf("\n=== 处理完成！===\n")
	fmt.Printf("总计处理: %d, 成功: %d, 错误: %d, 价格范围内: %d\n",
		rangeEnd-rangeStart+1, successCount, errorCount, priceRangeCount)
}
*/

func fetchGood(client *http.Client, apiToken string, id int) (*GoodResponse, error) {
	url := fmt.Sprintf("https://api.csqaq.com/api/v1/info/good?id=%d", id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if apiToken != "" {
		req.Header.Set("ApiToken", apiToken)
	}
	req.Header.Set("Content-Type", "application/json")

	maxRetries := 3
	for retry := 0; retry <= maxRetries; retry++ {
		resp, err := client.Do(req)
		if err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*2000) * time.Millisecond)
				continue
			}
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*2000) * time.Millisecond)
				continue
			}
			return nil, err
		}

		// 检查429错误
		if strings.Contains(string(body), "429 Too Many Requests") {
			if retry < maxRetries {
				backoffTime := (retry + 1) * 8000 // 8秒、16秒、24秒递增退避
				fmt.Printf("限流 good_id %d, 重试 %d/%d (等待 %d 毫秒)\n", id, retry+1, maxRetries, backoffTime)
				time.Sleep(time.Duration(backoffTime) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("rate limited after retries")
		}

		// 检查HTML响应
		if strings.Contains(string(body), "<!DOCTYPE html>") {
			if retry < maxRetries {
				fmt.Printf("Good ID %d 返回HTML, 重试 %d/%d\n", id, retry+1, maxRetries)
				time.Sleep(time.Duration((retry+2)*3000) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("API返回HTML页面")
		}

		var goodResp GoodResponse
		if err := json.Unmarshal(body, &goodResp); err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*2000) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("json parse error: %v", err)
		}

		if goodResp.Code != 200 || goodResp.Data.GoodsInfo.ID == 0 {
			return nil, nil // 商品不存在
		}

		return &goodResp, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}
