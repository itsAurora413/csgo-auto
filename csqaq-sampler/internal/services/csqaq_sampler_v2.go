package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"csqaq-sampler/internal/models"

	"gorm.io/gorm"
)

// EnhancedCSQAQSampler provides precise interval control and high success rate
type EnhancedCSQAQSampler struct {
	db           *gorm.DB
	get          func(endpoint string, params map[string]string) ([]byte, error)
	interval     time.Duration // 1.6s = 1600ms
	maxRetries   int
	retryBackoff time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	stats        SamplerStats
	running      bool
	currentIndex int                // 当前处理的商品索引
	allGoods     []models.CSQAQGood // 所有商品列表
	maxGoods     int                // 最大商品数限制，0表示无限制
}

type SamplerStats struct {
	TotalRequests   int64     `json:"total_requests"`
	SuccessRequests int64     `json:"success_requests"`
	FailedRequests  int64     `json:"failed_requests"`
	RetryCount      int64     `json:"retry_count"`
	SuccessRate     float64   `json:"success_rate"`
	AvgResponseTime float64   `json:"avg_response_time_ms"`
	LastRun         time.Time `json:"last_run"`
	ValidPriceCount int64     `json:"valid_price_count"`
}

type goodResp struct {
    Code int64 `json:"code"`
    Data struct {
        GoodsInfo struct {
            YyypSellPrice float64 `json:"yyyp_sell_price"`
            YyypBuyPrice  float64 `json:"yyyp_buy_price"`
            YyypSellNum   int     `json:"yyyp_sell_num"` // 悠悠有品在售数量
            YyypBuyNum    int     `json:"yyyp_buy_num"`  // 悠悠有品求购数量
            BuffSellPrice float64 `json:"buff_sell_price"`
            BuffBuyPrice  float64 `json:"buff_buy_price"`
            YyypID         int64   `json:"yyyp_id"`
        } `json:"goods_info"`
    } `json:"data"`
}

// NewEnhancedCSQAQSampler creates a new enhanced sampler with precise timing
func NewEnhancedCSQAQSampler(db *gorm.DB, get func(endpoint string, params map[string]string) ([]byte, error)) *EnhancedCSQAQSampler {
	ctx, cancel := context.WithCancel(context.Background())
	return &EnhancedCSQAQSampler{
		db:           db,
		get:          get,
		interval:     1600 * time.Millisecond, // 精确1.6秒
		maxRetries:   3,
		retryBackoff: 500 * time.Millisecond,
		ctx:          ctx,
		cancel:       cancel,
		stats:        SamplerStats{},
		currentIndex: 0,
		allGoods:     []models.CSQAQGood{},
		maxGoods:     0, // 0表示无限制，处理所有商品
	}
}

// Start begins the continuous sampling process
func (s *EnhancedCSQAQSampler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	fmt.Printf("[Enhanced CSQAQ Sampler] Starting continuous sampling with %v intervals\n", s.interval)

	go func() {
		// Initial delay
		time.Sleep(5 * time.Second)
		fmt.Printf("[Enhanced CSQAQ Sampler] Initial delay completed, starting continuous loop\n")

		// Load all goods initially
		if err := s.loadAllGoods(); err != nil {
			fmt.Printf("[Enhanced CSQAQ Sampler] Failed to load goods: %v\n", err)
			return
		}

		for {
			select {
			case <-s.ctx.Done():
				fmt.Printf("[Enhanced CSQAQ Sampler] Context cancelled, stopping\n")
				return
			default:
				if err := s.processNextGood(); err != nil {
					fmt.Printf("[Enhanced CSQAQ Sampler] Error processing good: %v\n", err)
					// Wait a bit before continuing on error
					time.Sleep(s.interval)
				} else {
					// Wait the specified interval before next request
					time.Sleep(s.interval)
				}
			}
		}
	}()
}

// Stop stops the enhanced sampler
func (s *EnhancedCSQAQSampler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.cancel()
		s.running = false
		fmt.Printf("[Enhanced CSQAQ Sampler] Stopped\n")
	}
}

// GetStats returns current sampler statistics
func (s *EnhancedCSQAQSampler) GetStats() SamplerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	if stats.TotalRequests > 0 {
		stats.SuccessRate = float64(stats.SuccessRequests) / float64(stats.TotalRequests) * 100
	}
	return stats
}

// loadAllGoods loads all goods from database
func (s *EnhancedCSQAQSampler) loadAllGoods() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var goods []models.CSQAQGood
	if err := s.db.Model(&models.CSQAQGood{}).Order("good_id asc").Find(&goods).Error; err != nil {
		return fmt.Errorf("failed to load goods: %w", err)
	}

	s.allGoods = goods
	s.currentIndex = 0
	fmt.Printf("[Enhanced CSQAQ Sampler] Loaded %d goods for continuous processing\n", len(goods))
	return nil
}

// processNextGood processes the next good in sequence
func (s *EnhancedCSQAQSampler) processNextGood() error {
	s.mu.Lock()
	if len(s.allGoods) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("no goods loaded")
	}

	// Check if we need to restart the cycle
	if s.currentIndex >= len(s.allGoods) {
		fmt.Printf("[Enhanced CSQAQ Sampler] Completed full cycle of %d goods, restarting from beginning\n", len(s.allGoods))
		s.currentIndex = 0

		// Reload goods to get any new ones
		s.mu.Unlock()
		if err := s.loadAllGoods(); err != nil {
			return fmt.Errorf("failed to reload goods: %w", err)
		}
		s.mu.Lock()
	}

	good := s.allGoods[s.currentIndex]
	currentPos := s.currentIndex + 1
	totalGoods := len(s.allGoods)
	s.currentIndex++
	s.mu.Unlock()

	// Process the good
	var retryCount int64
	success, responseTime := s.processGoodWithRetry(good.GoodID, &retryCount)

	// Update statistics
	s.mu.Lock()
	s.stats.TotalRequests++
	if success {
		s.stats.SuccessRequests++
		if s.isValidPriceRange(good.GoodID) {
			s.stats.ValidPriceCount++
		}
	} else {
		s.stats.FailedRequests++
	}
	s.stats.RetryCount += retryCount
	s.stats.LastRun = time.Now()
	s.stats.AvgResponseTime = float64(responseTime.Nanoseconds()) / 1000000
	s.mu.Unlock()

	// Progress reporting for each good
	successRate := float64(s.stats.SuccessRequests) / float64(s.stats.TotalRequests) * 100
	fmt.Printf("[Enhanced CSQAQ Sampler] Progress: %d/%d processed, %d valid prices, %.1f%% success rate\n",
		currentPos, totalGoods, s.stats.ValidPriceCount, successRate)

	return nil
}

// runOnceEnhanced runs one sampling cycle with enhanced error handling and precise timing
func (s *EnhancedCSQAQSampler) runOnceEnhanced() error {
	startTime := time.Now()
	fmt.Printf("[Enhanced CSQAQ Sampler] Starting enhanced sampling cycle\n")

	var goods []models.CSQAQGood
	q := s.db.Model(&models.CSQAQGood{}).Order("updated_at desc")
	if s.maxGoods > 0 {
		q = q.Limit(s.maxGoods * 5) // 获取更多用于筛选
	}

	if err := q.Find(&goods).Error; err != nil {
		fmt.Printf("[Enhanced CSQAQ Sampler] Database query error: %v\n", err)
		return err
	}

	if len(goods) == 0 {
		fmt.Printf("[Enhanced CSQAQ Sampler] No goods found, skipping\n")
		return nil
	}

	fmt.Printf("[Enhanced CSQAQ Sampler] Found %d goods, processing with 1.6s intervals\n", len(goods))

	var (
		successCount      int64
		errorCount        int64
		retryCount        int64
		validPriceCount   int64
		totalResponseTime time.Duration
	)

	// 精确间隔控制
	nextRequestTime := time.Now()

	for i, g := range goods {
		if int64(validPriceCount) >= int64(s.maxGoods) && s.maxGoods > 0 {
			fmt.Printf("[Enhanced CSQAQ Sampler] Reached target of %d goods, stopping\n", s.maxGoods)
			break
		}

		// 检查是否应该停止
		select {
		case <-s.ctx.Done():
			fmt.Printf("[Enhanced CSQAQ Sampler] Context cancelled during processing\n")
			return s.ctx.Err()
		default:
		}

		// 精确等待到下次请求时间
		if i > 0 {
			sleepDuration := time.Until(nextRequestTime)
			if sleepDuration > 0 {
				time.Sleep(sleepDuration)
			}
		}

		requestStart := time.Now()

		// 带重试的API调用
		success, responseTime := s.processGoodWithRetry(g.GoodID, &retryCount)
		totalResponseTime += responseTime

		if success {
			successCount++
			// 只有成功获取到有效价格的才计入validPriceCount
			if s.isValidPriceRange(g.GoodID) {
				validPriceCount++
			}
		} else {
			errorCount++
		}

		// 设置下次请求的精确时间
		nextRequestTime = requestStart.Add(s.interval)

		// 进度报告 - 每个商品都报告
		fmt.Printf("[Enhanced CSQAQ Sampler] Progress: %d/%d processed, %d valid prices, %.1f%% success rate\n",
			i+1, len(goods), validPriceCount, float64(successCount)/float64(successCount+errorCount)*100)
	}

	// 更新统计信息
	s.mu.Lock()
	s.stats.TotalRequests += successCount + errorCount
	s.stats.SuccessRequests += successCount
	s.stats.FailedRequests += errorCount
	s.stats.RetryCount += retryCount
	s.stats.ValidPriceCount += validPriceCount
	s.stats.LastRun = time.Now()
	if successCount+errorCount > 0 {
		s.stats.AvgResponseTime = float64(totalResponseTime.Nanoseconds()) / float64(successCount+errorCount) / 1000000
	}
	s.mu.Unlock()

	duration := time.Since(startTime)
	successRate := float64(successCount) / float64(successCount+errorCount) * 100

	fmt.Printf("[Enhanced CSQAQ Sampler] Cycle completed in %v: %d success, %d errors, %d retries, %d valid prices, %.1f%% success rate\n",
		duration, successCount, errorCount, retryCount, validPriceCount, successRate)

	return nil
}

// processGoodWithRetry processes a single good with retry mechanism
func (s *EnhancedCSQAQSampler) processGoodWithRetry(goodID int64, retryCount *int64) (bool, time.Duration) {
	requestStart := time.Now()

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			// 递增重试延迟
			retryDelay := time.Duration(attempt) * s.retryBackoff
			time.Sleep(retryDelay)
			*retryCount++
		}

		body, err := s.get("info/good", map[string]string{"id": fmt.Sprintf("%d", goodID)})
		if err != nil {
			if attempt == s.maxRetries {
				fmt.Printf("[Enhanced CSQAQ Sampler] Final API error for good_id %d: %v\n", goodID, err)
				return false, time.Since(requestStart)
			}
			continue
		}

		var gr goodResp
		if err := json.Unmarshal(body, &gr); err != nil {
			if attempt == s.maxRetries {
				fmt.Printf("[Enhanced CSQAQ Sampler] JSON error for good_id %d: %v\n", goodID, err)
				return false, time.Since(requestStart)
			}
			continue
		}

		if gr.Code != 200 {
			if attempt == s.maxRetries {
				fmt.Printf("[Enhanced CSQAQ Sampler] API code error for good_id %d: %d\n", goodID, gr.Code)
				return false, time.Since(requestStart)
			}
			continue
		}

		// 成功获取数据，保存快照
		if s.saveSnapshot(goodID, gr.Data.GoodsInfo) {
			return true, time.Since(requestStart)
		} else {
			return false, time.Since(requestStart)
		}
	}

	return false, time.Since(requestStart)
}

// saveSnapshot saves price snapshot to database with retry
func (s *EnhancedCSQAQSampler) saveSnapshot(goodID int64, gi struct {
    YyypSellPrice float64 `json:"yyyp_sell_price"`
    YyypBuyPrice  float64 `json:"yyyp_buy_price"`
    YyypSellNum   int     `json:"yyyp_sell_num"`
    YyypBuyNum    int     `json:"yyyp_buy_num"`
    BuffSellPrice float64 `json:"buff_sell_price"`
    BuffBuyPrice  float64 `json:"buff_buy_price"`
    YyypID         int64   `json:"yyyp_id"`
}) bool {
    snap := models.CSQAQGoodSnapshot{
        GoodID:    goodID,
        CreatedAt: time.Now(),
    }

	// Convert to pointers for nullable fields
	yyypSell := gi.YyypSellPrice
	snap.YYYPSellPrice = &yyypSell
	yyypBuy := gi.YyypBuyPrice
	snap.YYYPBuyPrice = &yyypBuy
	yyypSellCount := gi.YyypSellNum
	snap.YYYPSellCount = &yyypSellCount
	yyypBuyCount := gi.YyypBuyNum
	snap.YYYPBuyCount = &yyypBuyCount
	buffSell := gi.BuffSellPrice
	snap.BuffSellPrice = &buffSell
    buffBuy := gi.BuffBuyPrice
    snap.BuffBuyPrice = &buffBuy

    // Set YouPin id (yyyp_id from API) into snapshot.YYYPTemplateID
    if gi.YyypID > 0 {
        tpl := gi.YyypID
        snap.YYYPTemplateID = &tpl
    }

	// 数据库操作重试
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.db.Create(&snap).Error; err != nil {
			if attempt == 2 {
				fmt.Printf("[Enhanced CSQAQ Sampler] Database error for good_id %d: %v\n", goodID, err)
				return false
			}
			time.Sleep(100 * time.Millisecond)
		} else {
			return true
		}
	}
	return false
}

// isValidPriceRange checks if the good has prices in 50-300 range
func (s *EnhancedCSQAQSampler) isValidPriceRange(goodID int64) bool {
	var snap models.CSQAQGoodSnapshot
	err := s.db.Where("good_id = ?", goodID).Order("created_at desc").First(&snap).Error
	if err != nil {
		return false
	}

	if snap.YYYPSellPrice != nil && *snap.YYYPSellPrice >= 50 && *snap.YYYPSellPrice <= 300 {
		return true
	}
	if snap.BuffSellPrice != nil && *snap.BuffSellPrice >= 50 && *snap.BuffSellPrice <= 300 {
		return true
	}
	return false
}

// StartEnhancedCSQAQSampler starts the enhanced sampler with continuous processing
func StartEnhancedCSQAQSampler(db *gorm.DB, get func(endpoint string, params map[string]string) ([]byte, error)) *EnhancedCSQAQSampler {
	sampler := NewEnhancedCSQAQSampler(db, get)
	sampler.Start()
	return sampler
}
