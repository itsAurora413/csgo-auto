package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"csqaq-sampler/internal/models"
	"gorm.io/gorm"
)

// MultiThreadedSampler wraps the basic sampler with proxy-based multithreading
type MultiThreadedSampler struct {
	db              *gorm.DB
	baseGetFunc     func(endpoint string, params map[string]string) ([]byte, error)
	proxyConfig     *ProxyConfig
	numWorkers      int
	workerPool      *ProxyWorkerPool
	interval        time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex
	running         bool
	stats           MTSamplerStats
	maxRetries      int
	retryBackoff    time.Duration
}

// MTSamplerStats tracks statistics for multithreaded sampler
type MTSamplerStats struct {
	TotalProcessed   int64
	SuccessRequests  int64
	FailedRequests   int64
	BindErrors       int64
	ValidSnapshots   int64
	LastRun          time.Time
	TotalDuration    time.Duration
	AvgResponseTime  float64
}

// NewMultiThreadedSampler creates a new multithreaded sampler
func NewMultiThreadedSampler(
	db *gorm.DB,
	baseGetFunc func(endpoint string, params map[string]string) ([]byte, error),
	proxyConfig *ProxyConfig,
	numWorkers int,
	apiKey string,
) (*MultiThreadedSampler, error) {

	if numWorkers <= 0 {
		numWorkers = 5
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create proxy worker pool
	workerPool, err := NewProxyWorkerPool(numWorkers, proxyConfig, apiKey, baseGetFunc)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create worker pool: %w", err)
	}

	sampler := &MultiThreadedSampler{
		db:          db,
		baseGetFunc: baseGetFunc,
		proxyConfig: proxyConfig,
		numWorkers:  numWorkers,
		workerPool:  workerPool,
		interval:    200 * time.Millisecond, // 200ms interval for API compliance
		ctx:         ctx,
		cancel:      cancel,
		running:     false,
		maxRetries:  3,
		retryBackoff: 500 * time.Millisecond,
	}

	return sampler, nil
}

// Start begins continuous multithreaded sampling
func (s *MultiThreadedSampler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Printf("[多线程采样器] 启动，%d 个工作线程，采样间隔 %v\n", s.numWorkers, s.interval)

	go s.samplingLoop()
}

// samplingLoop is the main sampling loop
func (s *MultiThreadedSampler) samplingLoop() {
	// Initial delay
	time.Sleep(5 * time.Second)
	log.Println("[多线程采样器] 初始延迟完成，开始采样")

	for {
		select {
		case <-s.ctx.Done():
			log.Println("[多线程采样器] 收到停止信号")
			return
		default:
			if err := s.runSamplingCycle(); err != nil {
				log.Printf("[多线程采样器] 采样周期错误: %v\n", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// runSamplingCycle runs one complete sampling cycle using worker pool
func (s *MultiThreadedSampler) runSamplingCycle() error {
	startTime := time.Now()

	// Load all goods from database
	var goods []models.CSQAQGood
	if err := s.db.Model(&models.CSQAQGood{}).Order("updated_at desc").Find(&goods).Error; err != nil {
		return fmt.Errorf("failed to load goods: %w", err)
	}

	if len(goods) == 0 {
		log.Println("[多线程采样器] 未找到商品，跳过本周期")
		return nil
	}

	log.Printf("[多线程采样器] 开始处理 %d 个商品 (使用 %d 个工作线程)\n", len(goods), s.numWorkers)

	var (
		successCount   int64
		failureCount   int64
		validSnapshots int64
		totalDuration  time.Duration
		mu             sync.Mutex
	)

	// 使用 WaitGroup 同步所有goroutine
	var wg sync.WaitGroup

	// Goroutine 1: 任务分发 (主线程速率控制)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, good := range goods {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// 向工作线程池提交任务
			taskID := fmt.Sprintf("good_%d_%d", good.GoodID, time.Now().UnixNano())
			task := &WorkerTask{
				TaskID:   taskID,
				GoodID:   good.GoodID,
				Endpoint: "info/good",
				Params:   map[string]string{"id": fmt.Sprintf("%d", good.GoodID)},
			}

			if err := s.workerPool.SubmitTask(task); err != nil {
				log.Printf("[多线程采样器] 提交任务失败 (good_id %d): %v\n", good.GoodID, err)
			}

			if (i + 1) % 100 == 0 {
				log.Printf("[多线程采样器] 已分发 %d 个任务\n", i+1)
			}

			// 速率控制 (200ms间隔)
			time.Sleep(s.interval)
		}
	}()

	// Goroutine 2: 结果收集 (从工作线程池读取结果)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < len(goods); i++ {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// 从工作线程池获取结果
			workerResult, err := s.workerPool.GetResult()
			if err != nil {
				log.Printf("[多线程采样器] 获取结果失败: %v\n", err)
				atomic.AddInt64(&failureCount, 1)
				continue
			}

			mu.Lock()
			totalDuration += workerResult.Duration
			mu.Unlock()

			if workerResult.Success && !workerResult.BindError {
				// 成功 - 解析并保存到数据库
				var gr goodResp
				if err := json.Unmarshal(workerResult.Data, &gr); err == nil && gr.Code == 200 {
					isValid := s.saveGoodSnapshot(workerResult.GoodID, gr.Data.GoodsInfo)
					atomic.AddInt64(&successCount, 1)
					if isValid {
						atomic.AddInt64(&validSnapshots, 1)
					}
				} else {
					atomic.AddInt64(&failureCount, 1)
				}
			} else {
				// 失败
				if workerResult.BindError {
					log.Printf("[多线程采样器] 工作线程 %d 绑定IP失败\n", workerResult.WorkerID)
				}
				atomic.AddInt64(&failureCount, 1)
			}

			if (i + 1) % 100 == 0 {
				log.Printf("[多线程采样器] 已收集 %d 个结果，成功 %d，失败 %d\n",
					i+1, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failureCount))
			}
		}
	}()

	// 等待所有goroutine完成
	wg.Wait()

	duration := time.Since(startTime)
	successCountFinal := atomic.LoadInt64(&successCount)
	failureCountFinal := atomic.LoadInt64(&failureCount)
	validSnapshotsFinal := atomic.LoadInt64(&validSnapshots)

	var successRate float64
	if successCountFinal+failureCountFinal > 0 {
		successRate = float64(successCountFinal) / float64(successCountFinal+failureCountFinal) * 100
	}

	// 更新统计
	s.mu.Lock()
	s.stats.TotalProcessed += int64(len(goods))
	s.stats.SuccessRequests += successCountFinal
	s.stats.FailedRequests += failureCountFinal
	s.stats.ValidSnapshots += validSnapshotsFinal
	s.stats.LastRun = time.Now()
	s.stats.TotalDuration += duration
	if successCountFinal+failureCountFinal > 0 {
		s.stats.AvgResponseTime = float64(totalDuration.Nanoseconds()) / float64(successCountFinal+failureCountFinal) / 1000000
	}
	s.mu.Unlock()

	log.Printf("[多线程采样器] 周期完成: 处理 %d 个，成功 %d，失败 %d，有效快照 %d，成功率 %.1f%%，耗时 %v\n",
		len(goods), successCountFinal, failureCountFinal, validSnapshotsFinal, successRate, duration)

	return nil
}

// SamplingResult holds the result of sampling one good
type SamplingResult struct {
	Success      bool
	GoodID       int64
	Duration     time.Duration
	IsValidPrice bool
	SnapshotID   int64
}

// saveGoodSnapshot saves a price snapshot to the database
func (s *MultiThreadedSampler) saveGoodSnapshot(goodID int64, gi struct {
	YyypSellPrice float64 `json:"yyyp_sell_price"`
	YyypBuyPrice  float64 `json:"yyyp_buy_price"`
	YyypSellNum   int     `json:"yyyp_sell_num"`
	YyypBuyNum    int     `json:"yyyp_buy_num"`
	BuffSellPrice float64 `json:"buff_sell_price"`
	BuffBuyPrice  float64 `json:"buff_buy_price"`
	YyypID        int64   `json:"yyyp_id"`
}) bool {
	snap := models.CSQAQGoodSnapshot{
		GoodID:    goodID,
		CreatedAt: time.Now(),
	}

	// Convert to pointers
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

	if gi.YyypID > 0 {
		tpl := gi.YyypID
		snap.YYYPTemplateID = &tpl
	}

	// 数据库保存重试
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.db.Create(&snap).Error; err != nil {
			if attempt == 2 {
				log.Printf("[多线程采样器] 数据库保存失败 (good_id %d): %v\n", goodID, err)
				return false
			}
			time.Sleep(100 * time.Millisecond)
		} else {
			return true
		}
	}
	return false
}

// GetStats returns current statistics
func (s *MultiThreadedSampler) GetStats() MTSamplerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Stop gracefully stops the sampler
func (s *MultiThreadedSampler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()

	// Shutdown worker pool
	if err := s.workerPool.Shutdown(30 * time.Second); err != nil {
		log.Printf("[多线程采样器] 关闭工作线程池出错: %v\n", err)
	}

	log.Println("[多线程采样器] 已停止")
}
