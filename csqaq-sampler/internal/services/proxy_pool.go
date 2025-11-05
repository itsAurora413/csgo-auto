package services

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyConfig holds proxy configuration for workers
type ProxyConfig struct {
	URL       string
	Username  string
	Password  string
	Timeout   time.Duration
	Enabled   bool
	BindInterval time.Duration // IP bind interval (default 35s)
}

// ProxyWorker represents a single worker thread with its own HTTP client and proxy IP
type ProxyWorker struct {
	id            int
	client        *http.Client
	proxyConfig   *ProxyConfig
	lastBindTime  time.Time
	mu            sync.Mutex
	requestCount  int64
	successCount  int64
	failureCount  int64
	bindFailCount int64
}

// ProxyWorkerPool manages multiple proxy workers for concurrent requests
type ProxyWorkerPool struct {
	workers       []*ProxyWorker
	taskQueue     chan *WorkerTask
	resultQueue   chan *WorkerResult
	wg            sync.WaitGroup
	ctx            chan struct{} // Stop signal
	apiKey        string
	numWorkers    int
	proxyConfig   *ProxyConfig
	getFunction   func(endpoint string, params map[string]string) ([]byte, error)
	stats         PoolStats
	statsMu       sync.RWMutex
}

// WorkerTask defines a task for a worker
type WorkerTask struct {
	TaskID   string
	GoodID   int64
	Endpoint string
	Params   map[string]string
}

// WorkerResult holds the result of a task
type WorkerResult struct {
	TaskID    string
	GoodID    int64
	Success   bool
	Data      []byte
	Error     string
	Duration  time.Duration
	WorkerID  int
	BindError bool // Indicates if bind operation failed
}

// PoolStats tracks statistics for the worker pool
type PoolStats struct {
	TotalTasks    int64
	SuccessTasks  int64
	FailedTasks   int64
	BindErrors    int64
	TotalDuration time.Duration
}

// NewProxyWorkerPool creates a new proxy worker pool
func NewProxyWorkerPool(numWorkers int, proxyConfig *ProxyConfig, apiKey string,
	getFunction func(endpoint string, params map[string]string) ([]byte, error)) (*ProxyWorkerPool, error) {

	if numWorkers <= 0 {
		numWorkers = 5 // Default to 5 workers
	}

	pool := &ProxyWorkerPool{
		workers:     make([]*ProxyWorker, numWorkers),
		taskQueue:   make(chan *WorkerTask, numWorkers*2), // Buffer size = 2x workers
		resultQueue: make(chan *WorkerResult, numWorkers*2),
		ctx:         make(chan struct{}),
		apiKey:      apiKey,
		numWorkers:  numWorkers,
		proxyConfig: proxyConfig,
		getFunction: getFunction,
	}

	// Create workers
	for i := 0; i < numWorkers; i++ {
		worker, err := NewProxyWorker(i, proxyConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create worker %d: %w", i, err)
		}
		pool.workers[i] = worker
	}

	// Start worker goroutines
	pool.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go pool.workerLoop(pool.workers[i])
	}

	log.Printf("[代理工作线程池] 已创建 %d 个工作线程\n", numWorkers)
	return pool, nil
}

// NewProxyWorker creates a new proxy worker
func NewProxyWorker(id int, proxyConfig *ProxyConfig) (*ProxyWorker, error) {
	var client *http.Client

	if proxyConfig.Enabled {
		// Create proxy-aware HTTP client with authentication
		proxyURLStr := fmt.Sprintf("http://%s:%s@%s", proxyConfig.Username, proxyConfig.Password, proxyConfig.URL)
		proxyURL, err := url.Parse(proxyURLStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}

		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}

		client = &http.Client{
			Transport: transport,
			Timeout:   proxyConfig.Timeout,
		}
	} else {
		// Create regular HTTP client without proxy
		client = &http.Client{Timeout: proxyConfig.Timeout}
	}

	return &ProxyWorker{
		id:           id,
		client:       client,
		proxyConfig:  proxyConfig,
		lastBindTime: time.Now().Add(-proxyConfig.BindInterval), // Start with old bind time to force first bind
	}, nil
}

// workerLoop is the main loop for each worker
func (pool *ProxyWorkerPool) workerLoop(worker *ProxyWorker) {
	defer pool.wg.Done()

	for {
		select {
		case <-pool.ctx:
			log.Printf("[工作线程-%d] 收到停止信号，退出\n", worker.id)
			return
		case task := <-pool.taskQueue:
			if task == nil {
				return
			}
			pool.processTask(worker, task)
		}
	}
}

// processTask processes a single task in a worker
func (pool *ProxyWorkerPool) processTask(worker *ProxyWorker, task *WorkerTask) {
	startTime := time.Now()

	// Bind IP if needed (every 35 seconds per worker)
	bindError := false
	if time.Since(worker.lastBindTime) >= pool.proxyConfig.BindInterval {
		if err := pool.bindIPViaProxy(worker); err != nil {
			log.Printf("[工作线程-%d] 绑定IP失败: %v\n", worker.id, err)
			bindError = true
			pool.statsMu.Lock()
			pool.stats.BindErrors++
			pool.statsMu.Unlock()
		} else {
			worker.lastBindTime = time.Now()
		}
	}

	// Make the actual request
	success := true
	var data []byte
	var errMsg string

	if task.Endpoint != "" && task.Params != nil {
		respData, err := pool.getFunction(task.Endpoint, task.Params)
		if err != nil {
			success = false
			errMsg = err.Error()
			data = nil
		} else {
			data = respData
		}
	}

	duration := time.Since(startTime)

	// Update worker stats
	worker.mu.Lock()
	worker.requestCount++
	if success && !bindError {
		worker.successCount++
	} else {
		worker.failureCount++
	}
	if bindError {
		worker.bindFailCount++
	}
	worker.mu.Unlock()

	// Update pool stats
	pool.statsMu.Lock()
	pool.stats.TotalTasks++
	if success && !bindError {
		pool.stats.SuccessTasks++
	} else {
		pool.stats.FailedTasks++
	}
	pool.stats.TotalDuration += duration
	pool.statsMu.Unlock()

	// Send result
	result := &WorkerResult{
		TaskID:    task.TaskID,
		GoodID:    task.GoodID,
		Success:   success && !bindError,
		Data:      data,
		Error:     errMsg,
		Duration:  duration,
		WorkerID:  worker.id,
		BindError: bindError,
	}

	select {
	case pool.resultQueue <- result:
	case <-pool.ctx:
		return
	}
}

// bindIPViaProxy binds IP through the proxy
// This is called at the start and every BindInterval
func (pool *ProxyWorkerPool) bindIPViaProxy(worker *ProxyWorker) error {
	// Bind IP via proxy - using the same CSQAQ API pattern
	// The sys/bind_local_ip endpoint doesn't require parameters
	params := make(map[string]string)
	_, err := pool.getFunction("sys/bind_local_ip", params)
	if err != nil {
		log.Printf("[工作线程-%d] 绑定IP失败: %v\n", worker.id, err)
		return err
	}
	log.Printf("[工作线程-%d] 成功绑定IP，将使用此IP进行 %.0f 秒\n", worker.id, pool.proxyConfig.BindInterval.Seconds())
	return nil
}

// ProcessGoodParallel processes a good with parallel worker pool
// This function coordinates the bind + fetch operation for each good
func (pool *ProxyWorkerPool) ProcessGoodParallel(goodID int64, endpoint string, params map[string]string) (*WorkerResult, error) {
	taskID := fmt.Sprintf("good_%d_%d", goodID, time.Now().UnixNano())
	task := &WorkerTask{
		TaskID:   taskID,
		GoodID:   goodID,
		Endpoint: endpoint,
		Params:   params,
	}

	if err := pool.SubmitTask(task); err != nil {
		return nil, err
	}

	result, err := pool.GetResult()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SubmitTask submits a task to the worker pool
func (pool *ProxyWorkerPool) SubmitTask(task *WorkerTask) error {
	select {
	case pool.taskQueue <- task:
		return nil
	case <-pool.ctx:
		return fmt.Errorf("pool is shutting down")
	}
}

// GetResult retrieves a result from the worker pool
func (pool *ProxyWorkerPool) GetResult() (*WorkerResult, error) {
	select {
	case result := <-pool.resultQueue:
		return result, nil
	case <-pool.ctx:
		return nil, fmt.Errorf("pool is shutting down")
	}
}

// GetStats returns the current pool statistics
func (pool *ProxyWorkerPool) GetStats() PoolStats {
	pool.statsMu.RLock()
	defer pool.statsMu.RUnlock()
	return pool.stats
}

// GetWorkerStats returns statistics for a specific worker
func (pool *ProxyWorkerPool) GetWorkerStats(workerID int) (map[string]int64, error) {
	if workerID < 0 || workerID >= len(pool.workers) {
		return nil, fmt.Errorf("invalid worker ID: %d", workerID)
	}

	worker := pool.workers[workerID]
	worker.mu.Lock()
	defer worker.mu.Unlock()

	stats := map[string]int64{
		"requests":     worker.requestCount,
		"successes":    worker.successCount,
		"failures":     worker.failureCount,
		"bind_errors":  worker.bindFailCount,
	}
	return stats, nil
}

// Shutdown gracefully shuts down the worker pool
func (pool *ProxyWorkerPool) Shutdown(timeout time.Duration) error {
	close(pool.ctx)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		pool.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[代理工作线程池] 已正常关闭所有 %d 个工作线程\n", pool.numWorkers)
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("pool shutdown timeout after %v", timeout)
	}
}
