package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// PredictionResult 预测结果
type PredictionResult struct {
	GoodID         int64                  `json:"good_id"`
	CurrentPrice   float64                `json:"current_price"`
	LastTimestamp  string                 `json:"last_timestamp"`
	ForecastDays   int                    `json:"forecast_days"`
	Predictions    map[string]interface{} `json:"predictions"`
	Ensemble       map[string]interface{} `json:"ensemble"`
	Recommendation map[string]interface{} `json:"recommendation"`
}

// Recommendation 建议信息
type Recommendation struct {
	Action          string  `json:"action"`  // buy, sell, hold
	NextPrice       float64 `json:"next_price"`
	PriceChangePct  float64 `json:"price_change_pct"`
	Reason          string  `json:"reason"`
	Confidence      float64 `json:"confidence"`
}

// PredictionClient 预测客户端
type PredictionClient struct {
	baseURL    string
	httpClient *http.Client
	cache      map[int64]*PredictionResult
	cacheLock  sync.RWMutex
	cacheTTL   time.Duration
}

// NewPredictionClient 创建预测客户端
func NewPredictionClient(baseURL string) *PredictionClient {
	return &PredictionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:    make(map[int64]*PredictionResult),
		cacheTTL: 1 * time.Hour,
	}
}

// Predict 预测单个商品
func (pc *PredictionClient) Predict(goodID int64, days int) (*PredictionResult, error) {
	if days < 1 || days > 30 {
		return nil, fmt.Errorf("预测天数必须在 1-30 之间, 收到: %d", days)
	}

	url := fmt.Sprintf("%s/api/predict/%d?days=%d", pc.baseURL, goodID, days)

	resp, err := pc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("预测请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("预测失败 (HTTP %d): %v", resp.StatusCode, errResp)
	}

	var result PredictionResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析预测结果失败: %w", err)
	}

	result.GoodID = goodID
	return &result, nil
}

// GetRecommendation 获取购买/卖出建议
func (pr *PredictionResult) GetRecommendation() (*Recommendation, error) {
	recData, ok := pr.Recommendation["recommendation"]
	if !ok {
		// 尝试直接从 map 获取
		if actionVal, hasAction := pr.Recommendation["action"]; hasAction {
			rec := &Recommendation{
				Action:     fmt.Sprintf("%v", actionVal),
				Confidence: 0.95,
			}

			if price, ok := pr.Recommendation["next_price"].(float64); ok {
				rec.NextPrice = price
			}
			if reason, ok := pr.Recommendation["reason"].(string); ok {
				rec.Reason = reason
			}
			if pc, ok := pr.Recommendation["price_change_pct"].(float64); ok {
				rec.PriceChangePct = pc
			}
			if conf, ok := pr.Recommendation["confidence"].(float64); ok {
				rec.Confidence = conf
			}

			return rec, nil
		}
		return nil, fmt.Errorf("推荐信息格式错误")
	}

	recDataMap, ok := recData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("推荐信息格式错误")
	}

	rec := &Recommendation{
		Action:     fmt.Sprintf("%v", recDataMap["action"]),
		NextPrice:  recDataMap["next_price"].(float64),
		Reason:     fmt.Sprintf("%v", recDataMap["reason"]),
		Confidence: recDataMap["confidence"].(float64),
	}

	if pc, ok := recDataMap["price_change_pct"].(float64); ok {
		rec.PriceChangePct = pc
	}

	return rec, nil
}

// GetEnsembleForecast 获取集成预测
func (pr *PredictionResult) GetEnsembleForecast() ([]float64, error) {
	ensembleData, ok := pr.Ensemble["forecast"]
	if !ok {
		return nil, fmt.Errorf("集成预测数据不存在")
	}

	forecastData, ok := ensembleData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("预测数据格式错误")
	}

	forecast := make([]float64, len(forecastData))
	for i, v := range forecastData {
		forecast[i] = v.(float64)
	}

	return forecast, nil
}

// GetXGBoostForecast 获取 XGBoost 预测
func (pr *PredictionResult) GetXGBoostForecast() ([]float64, error) {
	xgbData, ok := pr.Predictions["xgb"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("XGBoost 预测数据不存在")
	}

	forecastData, ok := xgbData["forecast"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("XGBoost 预测数据格式错误")
	}

	forecast := make([]float64, len(forecastData))
	for i, v := range forecastData {
		forecast[i] = v.(float64)
	}

	return forecast, nil
}

// BatchPredict 批量预测
func (pc *PredictionClient) BatchPredict(goodIDs []int64, days int) (map[int64]*PredictionResult, error) {
	if len(goodIDs) == 0 || len(goodIDs) > 100 {
		return nil, fmt.Errorf("商品数必须在 1-100 之间, 收到: %d", len(goodIDs))
	}

	url := fmt.Sprintf("%s/api/batch-predict", pc.baseURL)

	requestBody := map[string]interface{}{
		"good_ids": goodIDs,
		"days":     days,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := pc.httpClient.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("批量预测请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("批量预测失败 (HTTP %d): %v", resp.StatusCode, errResp)
	}

	var batchResp struct {
		TotalRequested int                 `json:"total_requested"`
		TotalSuccess   int                 `json:"total_success"`
		Results        []PredictionResult  `json:"results"`
	}

	if err := json.Unmarshal(body, &batchResp); err != nil {
		return nil, fmt.Errorf("解析批量结果失败: %w", err)
	}

	results := make(map[int64]*PredictionResult)
	for i := range batchResp.Results {
		goodID := batchResp.Results[i].GoodID
		results[goodID] = &batchResp.Results[i]
	}

	return results, nil
}

// ClearCache 清空服务器缓存
func (pc *PredictionClient) ClearCache() error {
	url := fmt.Sprintf("%s/api/clear-cache", pc.baseURL)

	resp, err := pc.httpClient.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("清空缓存请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("清空缓存失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// Health 健康检查
func (pc *PredictionClient) Health() (bool, error) {
	url := fmt.Sprintf("%s/api/health", pc.baseURL)

	resp, err := pc.httpClient.Get(url)
	if err != nil {
		return false, fmt.Errorf("健康检查请求失败: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// LocalCache 本地缓存 (可选)

// GetFromCache 从本地缓存获取
func (pc *PredictionClient) GetFromCache(goodID int64) (*PredictionResult, bool) {
	pc.cacheLock.RLock()
	defer pc.cacheLock.RUnlock()

	result, ok := pc.cache[goodID]
	return result, ok
}

// SetCache 设置本地缓存
func (pc *PredictionClient) SetCache(result *PredictionResult) {
	pc.cacheLock.Lock()
	defer pc.cacheLock.Unlock()

	pc.cache[result.GoodID] = result
}

// ClearLocalCache 清空本地缓存
func (pc *PredictionClient) ClearLocalCache() {
	pc.cacheLock.Lock()
	defer pc.cacheLock.Unlock()

	pc.cache = make(map[int64]*PredictionResult)
}
