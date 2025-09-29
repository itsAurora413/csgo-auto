package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
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

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("用法: %s <起始ID> <结束ID> [间隔毫秒]\n", os.Args[0])
		fmt.Printf("示例: %s 0 24041 1500\n", os.Args[0])
		os.Exit(1)
	}

	startID, _ := strconv.Atoi(os.Args[1])
	endID, _ := strconv.Atoi(os.Args[2])
	interval := 3000 // 默认3秒间隔，进一步避免频率限制
	if len(os.Args) > 3 {
		interval, _ = strconv.Atoi(os.Args[3])
	}

	fmt.Printf("开始获取饰品数据: ID %d 到 %d, 间隔 %d 毫秒\n", startID, endID, interval)

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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			good_id INTEGER UNIQUE NOT NULL,
			market_hash_name TEXT,
			name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS csqaq_good_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			good_id INTEGER NOT NULL,
			yyyp_sell_price REAL,
			buff_sell_price REAL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, sql := range createTables {
		_, err = db.Exec(sql)
		if err != nil {
			fmt.Printf("创建表失败: %v\n", err)
			os.Exit(1)
		}
	}

	// API令牌配置
	apiTokens := []string{
		"UAXMU177X578K1Q9E1G0N5M8", // 原有令牌
		"WPXHV1H7O5Y8N8W6R8U1N249", // 新令牌
	}

	totalSuccessCount := 0
	totalErrorCount := 0
	totalPriceRangeCount := 0
	processedCount := 0

	// 顺序执行两个令牌，避免频率冲突
	for i, token := range apiTokens {
		client := &http.Client{Timeout: 10 * time.Second}
		successCount := 0
		errorCount := 0
		priceRangeCount := 0

		fmt.Printf("API密钥 %d: %s\n", i+1, token)

		// 进行IP绑定
		fmt.Printf("令牌 %d 进行IP绑定...\n", i+1)
		if err := bindIP(client, token); err != nil {
			fmt.Printf("令牌 %d IP绑定失败: %v\n", i+1, err)
			fmt.Printf("令牌 %d 继续尝试API调用...\n", i+1)
		} else {
			fmt.Printf("令牌 %d IP绑定成功\n", i+1)
		}

		// 按范围分配ID：令牌0处理0-12020，令牌1处理12021-24041
		var rangeStart, rangeEnd int
		totalRange := endID - startID + 1
		rangeSize := totalRange / len(apiTokens)

		if i == 0 {
			rangeStart = startID
			rangeEnd = startID + rangeSize - 1
		} else {
			rangeStart = startID + rangeSize
			rangeEnd = endID
		}

		fmt.Printf("令牌 %d 处理范围: %d - %d\n", i+1, rangeStart, rangeEnd)

		for id := rangeStart; id <= rangeEnd; id++ {
			good, err := fetchGood(client, token, id)
			if err != nil {
				fmt.Printf("令牌 %d 获取 good_id %d 失败: %v\n", i+1, id, err)
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
				fmt.Printf("令牌 %d 跳过 good_id %d (%s), 价格超出范围: YYYP=%.2f, Buff=%.2f\n",
					i+1, goodInfo.ID, goodInfo.Name, goodInfo.YyypSellPrice, goodInfo.BuffSellPrice)
				time.Sleep(time.Duration(interval) * time.Millisecond)
				continue
			}

			// 保存商品信息到数据库
			// 这里不需要互斥锁，因为是顺序执行
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
					fmt.Printf("令牌 %d 保存快照 good_id %d 失败: %v\n", i+1, id, err2)
				}
			}

			processedCount++
			if processedCount%100 == 0 {
				fmt.Printf("进度: %d/%d, 成功: %d, 错误: %d, 价格范围内: %d\n",
					processedCount, endID-startID+1, totalSuccessCount, totalErrorCount, totalPriceRangeCount)
			}
			// 不需要 unlock

			if err != nil {
				fmt.Printf("令牌 %d 保存 good_id %d 失败: %v\n", i+1, id, err)
				errorCount++
			} else {
				if inPriceRange {
					fmt.Printf("令牌 %d ✓ 保存 good_id %d (%s), 价格: YYYP=%.2f, Buff=%.2f\n",
						i+1, goodInfo.ID, goodInfo.Name, goodInfo.YyypSellPrice, goodInfo.BuffSellPrice)
					priceRangeCount++
				} else {
					fmt.Printf("令牌 %d ✓ 保存 good_id %d (%s), 无价格数据\n", i+1, goodInfo.ID, goodInfo.Name)
				}
				successCount++
			}

			time.Sleep(time.Duration(interval) * time.Millisecond)
		}

		totalSuccessCount += successCount
		totalErrorCount += errorCount
		totalPriceRangeCount += priceRangeCount
		fmt.Printf("令牌 %d 完成！成功: %d, 错误: %d, 价格范围内: %d\n", i+1, successCount, errorCount, priceRangeCount)
	}
	fmt.Printf("\n全部完成！总计处理: %d, 成功: %d, 错误: %d, 价格范围内: %d\n",
		endID-startID+1, totalSuccessCount, totalErrorCount, totalPriceRangeCount)
}

func fetchGood(client *http.Client, apiToken string, id int) (*GoodResponse, error) {
	url := fmt.Sprintf("https://api.csqaq.com/api/v1/info/good?id=%d", id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置API密钥
	if apiToken != "" {
		req.Header.Set("ApiToken", apiToken)
	}
	req.Header.Set("Content-Type", "application/json")

	maxRetries := 3
	for retry := 0; retry <= maxRetries; retry++ {
		resp, err := client.Do(req)
		if err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*1000) * time.Millisecond)
				continue
			}
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*1000) * time.Millisecond)
				continue
			}
			return nil, err
		}

		// 检查是否返回HTML而不是JSON
		if strings.Contains(string(body), "<!DOCTYPE html>") {
			if retry < maxRetries {
				fmt.Printf("Good ID %d 返回HTML, 重试 %d/%d\n", id, retry+1, maxRetries)
				time.Sleep(time.Duration((retry+2)*2000) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("API返回HTML页面而不是JSON响应")
		}

		// 检查429错误
		if strings.Contains(string(body), "429 Too Many Requests") {
			if retry < maxRetries {
				backoffTime := (retry + 1) * 5000 // 5秒、10秒、15秒递增退避
				fmt.Printf("限流 good_id %d, 重试 %d/%d (等待 %d 毫秒)\n", id, retry+1, maxRetries, backoffTime)
				time.Sleep(time.Duration(backoffTime) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("rate limited after retries")
		}

		var goodResp GoodResponse
		if err := json.Unmarshal(body, &goodResp); err != nil {
			if retry < maxRetries {
				time.Sleep(time.Duration((retry+1)*1000) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("json parse error: %v", err)
		}

		if goodResp.Code != 200 || goodResp.Data.GoodsInfo.ID == 0 {
			// 商品不存在
			return nil, nil
		}

		return &goodResp, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

func bindIP(client *http.Client, apiToken string) error {
	url := "https://api.csqaq.com/api/v1/sys/bind_local_ip"

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	// 设置API密钥
	if apiToken != "" {
		req.Header.Set("ApiToken", apiToken)
	}
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

	fmt.Printf("IP绑定响应: %s\n", string(body))

	// 检查是否返回JSON响应
	if strings.Contains(string(body), "<!DOCTYPE html>") {
		return fmt.Errorf("API返回HTML页面而不是JSON响应")
	}

	// 检查绑定是否成功
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err == nil {
		if code, ok := response["code"].(float64); ok && code == 200 {
			return nil
		}
	}

	return fmt.Errorf("IP绑定失败，响应: %s", string(body))
}
