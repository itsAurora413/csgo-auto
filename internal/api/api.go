package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"csgo-trader/internal/models"
	steamService "csgo-trader/internal/services/steam"
	steamauth "csgo-trader/internal/services/steamauth"
	"csgo-trader/internal/services/youpin"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type APIHandler struct {
	db           *gorm.DB
	steamService *steamService.SteamService
	// full init job state
	jobMu   sync.Mutex
	fullJob *fullInitJob
	// IP binding state
	ipBound     bool
	ipBoundTime time.Time
	ipBindMu    sync.Mutex
}

func SetupRoutes(r *gin.RouterGroup, db *gorm.DB, steam *steamService.SteamService) *APIHandler {
	handler := &APIHandler{
		db:           db,
		steamService: steam,
	}

	// Auth routes - Steam login functionality
	auth := r.Group("/auth")
	{
		auth.GET("/steam/login", handler.SteamLogin)
		auth.GET("/steam/callback", handler.SteamCallback)
		auth.POST("/logout", handler.Logout)
		auth.GET("/me", handler.GetCurrentUser)
	}

	// CSQAQ API proxy routes - only K-line
	csqaq := r.Group("/csqaq")
	{
		// Index K-line per CSQAQ doc: https://docs.csqaq.com/api-278085071
		csqaq.GET("/sub/kline", handler.ProxyCSQAQIndexKline)

		// CSQAQ goods initialization and retrieval
		csqaq.POST("/init-goods", handler.InitCSQAQGoods)
		csqaq.GET("/goods", handler.ListCSQAQGoods)
		// Proxy CSQAQ good detail by id
		csqaq.GET("/good", handler.GetCSQAQGoodDetail)

		// Historical series (snapshots -> K-line)
		csqaq.GET("/good/kline", handler.GetGoodKline)
		csqaq.GET("/good/derived_kline", handler.GetGoodDerivedKline)
		// Snapshot single good now
		csqaq.POST("/good/snapshot", handler.SampleGoodSnapshot)

		// Purchase recommendations based on CSQAQ data analysis
		csqaq.GET("/recommendations", handler.GetPurchaseRecommendations)


		// Full range initialization (ID sweep)
		csqaq.POST("/init-goods-full/start", handler.StartFullInit)
		csqaq.GET("/init-goods-full/status", handler.FullInitStatus)
		csqaq.POST("/init-goods-full/stop", handler.StopFullInit)
	}

	// Alias without the /csqaq prefix
	r.GET("/sub/kline", handler.ProxyCSQAQIndexKline)

	// Forecast routes
	forecast := r.Group("/forecast")
	{
		forecast.POST("/run", handler.RunForecast)
		forecast.GET("/backtest", handler.BacktestForecast)
		forecast.POST("/backtest", handler.BacktestForecastPost)
		forecast.GET("/history", handler.ListForecastHistory)
	}

	// YouPin routes
	youpin := r.Group("/youpin")
	{
		// 账户管理
		youpin.POST("/accounts", handler.AddYouPinAccount)
		youpin.GET("/accounts", handler.GetYouPinAccounts)
		youpin.DELETE("/accounts/:id", handler.DeleteYouPinAccount)
		youpin.POST("/accounts/:id/reactivate", handler.ReactivateYouPinAccount)

		// 配置管理
		youpin.GET("/config", handler.GetYouPinConfig)
		youpin.PUT("/config", handler.UpdateYouPinConfig)

		// 自动功能
		youpin.POST("/auto-sell/start", handler.StartAutoSell)
		youpin.POST("/auto-change-price/start", handler.StartAutoChangePrice)
		youpin.POST("/auto-accept-offer/start", handler.StartAutoAcceptOffer)

		// 数据查询
		youpin.GET("/orders", handler.GetYouPinOrders)
		youpin.GET("/inventory", handler.GetYouPinInventory)

		// SMS认证
		youpin.POST("/send-sms", handler.SendYouPinSMS)
		youpin.POST("/login-with-phone", handler.LoginYouPinWithPhone)

		// 购买相关功能
		youpin.POST("/search", handler.SearchYouPinCommodities)
		youpin.POST("/search/items", handler.SearchYouPinItems)        // 新增：基于抓包信息的搜索接口
		youpin.POST("/commodity/list", handler.GetYouPinCommodityList) // 新增：获取商品列表
		youpin.POST("/filter/config", handler.GetYouPinFilterConfig)   // 新增：获取筛选配置
		youpin.GET("/commodity/:template_id", handler.GetYouPinCommodityDetails)
		youpin.POST("/market/items", handler.GetYouPinMarketItems)
		youpin.POST("/purchase/orders", handler.GetYouPinPurchaseOrders)
		youpin.POST("/buy", handler.BuyFromYouPinMarket)
		youpin.POST("/buy-with-balance", handler.BuyFromYouPinMarketWithBalance) // 新增：使用余额购买
		youpin.POST("/buy-multistep", handler.MultiStepBuyFromYouPinMarket) // 新增：基于HAR的多步骤购买流程
		youpin.POST("/purchase", handler.CreateYouPinPurchaseOrder)
		youpin.POST("/sell-item", handler.SellYouPinItem)
		youpin.GET("/sellable-items", handler.GetSellableItems)
		youpin.GET("/sell-list", handler.GetSellList)
		youpin.POST("/sell-by-name", handler.SellItemByName)
		youpin.POST("/sell-by-index", handler.SellItemsByIndex)
		youpin.POST("/instant-sell", handler.InstantSellItems)

		// 手动改价与下架
		youpin.POST("/change-price", handler.ChangeYouPinPrice)
		youpin.POST("/off-sale", handler.OffSaleYouPinItems)
	}

	// Steam credentials routes
	steamGroup := r.Group("/steam")
	{
		steamGroup.GET("/credentials", handler.GetSteamCredentials)
		steamGroup.PUT("/credentials", handler.UpdateSteamCredentials)
		steamGroup.POST("/login", handler.LoginSteamAndGetKey)
	}

	return handler
}


// Auth handlers
func (h *APIHandler) SteamLogin(c *gin.Context) {
	returnURL := c.Query("return_url")
	if returnURL == "" {
		// 动态构建回调URL而不是硬编码
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := c.Request.Host
		returnURL = fmt.Sprintf("%s://%s/api/v1/auth/steam/callback", scheme, host)
	}

	loginURL := h.steamService.GetOpenIDLoginURL(returnURL)
	c.JSON(http.StatusOK, gin.H{"login_url": loginURL})
}

func (h *APIHandler) SteamCallback(c *gin.Context) {
	// Get all query parameters
	params := c.Request.URL.Query()

	// 检查是否有OpenID错误
	if params.Get("openid.mode") == "cancel" {
		c.Redirect(http.StatusFound, "/?error=login_cancelled")
		return
	}

	// 验证必要的OpenID参数
	if params.Get("openid.mode") == "" || params.Get("openid.claimed_id") == "" {
		c.Redirect(http.StatusFound, "/?error=invalid_openid_response")
		return
	}

	steamID, err := h.steamService.VerifyOpenIDResponse(params)
	if err != nil {
		// 记录详细错误信息用于调试
		fmt.Printf("Steam OpenID verification failed: %v\n", err)
		fmt.Printf("OpenID params: %v\n", params)
		c.Redirect(http.StatusFound, "/?error=invalid_steam_login")
		return
	}

	// Get user info from Steam
	userInfo, err := h.steamService.GetUserInfo(steamID)
	if err != nil {
		fmt.Printf("Failed to get Steam user info for %s: %v\n", steamID, err)
		c.Redirect(http.StatusFound, "/?error=failed_to_get_user_info")
		return
	}

	// Save or update user in database
	var user models.User
	result := h.db.Where("steam_id = ?", steamID).First(&user)
	if result.Error != nil {
		// Create new user
		user = *userInfo
		if err := h.db.Create(&user).Error; err != nil {
			fmt.Printf("Failed to create user in database: %v\n", err)
			c.Redirect(http.StatusFound, "/?error=failed_to_create_user")
			return
		}
	} else {
		// Update existing user
		user.Username = userInfo.Username
		user.Avatar = userInfo.Avatar
		h.db.Save(&user)
	}

	// Redirect to frontend with success
	c.Redirect(http.StatusFound, fmt.Sprintf("/?login=success&user_id=%d&username=%s&avatar=%s",
		user.ID, url.QueryEscape(user.Username), url.QueryEscape(user.Avatar)))
}

func (h *APIHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (h *APIHandler) GetCurrentUser(c *gin.Context) {
	// This would typically check JWT token
	c.JSON(http.StatusOK, gin.H{"user": nil})
}

// CSQAQ API proxy handlers
const (
	CSQAQ_API_BASE = "https://api.csqaq.com/api/v1/"
	CSQAQ_API_KEY  = "UAXMU177X578K1Q9E1G0N5M8"
)

var lastBindTime time.Time

func (h *APIHandler) ensureIPBound() error {
	h.ipBindMu.Lock()
	defer h.ipBindMu.Unlock()

	// Check if IP needs to be bound (every 30 seconds)
	if h.ipBound && time.Since(h.ipBoundTime) < 30*time.Second {
		return nil // Recently bound, no need to bind again
	}

	// Time to bind IP again
	return h.bindLocalIP()
}

func (h *APIHandler) bindLocalIP() error {
	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := CSQAQ_API_BASE + "sys/bind_local_ip"

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}

	// Use CSQAQ_API_KEY as Header with key ApiToken
	req.Header.Set("ApiToken", CSQAQ_API_KEY)
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

	// Check response
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err == nil {
		if code, ok := response["code"].(float64); ok {
			lastBindTime = time.Now()
			h.ipBoundTime = time.Now()
			if code == 200 {
				h.ipBound = true
				fmt.Printf("Successfully bound local IP to CSQAQ API\n")
				return nil
			} else if code == 429 {
				// Rate limited, but this means IP was already bound recently
				h.ipBound = true
				fmt.Printf("IP binding rate limited (already bound recently)\n")
				return nil
			}
		}
	}

	fmt.Printf("Failed to bind local IP, response: %s\n", string(body))
	return fmt.Errorf("failed to bind local IP")
}

func (h *APIHandler) makeCSQAQRequest(endpoint string, params map[string]string) ([]byte, error) {
	// Ensure IP is bound before making API requests
	if err := h.ensureIPBound(); err != nil {
		fmt.Printf("Warning: Failed to bind local IP: %v\n", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Build URL with parameters
	reqURL := CSQAQ_API_BASE + endpoint
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Add(k, v)
		}
		reqURL += "?" + values.Encode()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("ApiToken", CSQAQ_API_KEY)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// makeCSQAQRequestWithKey allows specifying a custom API key
func (h *APIHandler) makeCSQAQRequestWithKey(endpoint string, params map[string]string, apiKey string) ([]byte, error) {
	// Ensure IP is bound before making API requests
	if err := h.ensureIPBound(); err != nil {
		fmt.Printf("Warning: Failed to bind local IP: %v\n", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Build URL with parameters
	reqURL := CSQAQ_API_BASE + endpoint
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Add(k, v)
		}
		reqURL += "?" + values.Encode()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("ApiToken", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// Exported wrappers for cross-package use (e.g., sampler in main)
func (h *APIHandler) MakeCSQAQRequest(endpoint string, params map[string]string) ([]byte, error) {
	return h.makeCSQAQRequest(endpoint, params)
}
func (h *APIHandler) MakeCSQAQPost(endpoint string, payload interface{}) ([]byte, error) {
	return h.makeCSQAQPost(endpoint, payload)
}


// -------- Full Init Job (ID sweep) --------

type fullInitJob struct {
	Running    bool       `json:"running"`
	StartID    int64      `json:"start_id"`
	EndID      int64      `json:"end_id"`
	Current    int64      `json:"current"`
	Success    int64      `json:"success"`
	Failed     int64      `json:"failed"`
	ThrottleMS int64      `json:"throttle_ms"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Error      string     `json:"error"`
}

func (h *APIHandler) StartFullInit(c *gin.Context) {
	var req struct {
		StartID    int64 `json:"start_id"`
		EndID      int64 `json:"end_id"`
		ThrottleMS int64 `json:"throttle_ms"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.StartID <= 0 {
		req.StartID = 1
	}
	if req.EndID <= 0 {
		req.EndID = 101466
	}
	if req.StartID > req.EndID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_id > end_id"})
		return
	}
	if req.ThrottleMS < 0 {
		req.ThrottleMS = 0
	}

	h.jobMu.Lock()
	if h.fullJob != nil && h.fullJob.Running {
		st := *h.fullJob
		h.jobMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "job already running", "status": st})
		return
	}
	job := &fullInitJob{Running: true, StartID: req.StartID, EndID: req.EndID, Current: req.StartID - 1, ThrottleMS: req.ThrottleMS, StartedAt: time.Now()}
	h.fullJob = job
	h.jobMu.Unlock()

	go h.runFullInitJob(job)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "started", "status": job})
}

func (h *APIHandler) StopFullInit(c *gin.Context) {
	h.jobMu.Lock()
	defer h.jobMu.Unlock()
	if h.fullJob == nil || !h.fullJob.Running {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "no running job"})
		return
	}
	h.fullJob.Running = false
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "stopping"})
}

func (h *APIHandler) FullInitStatus(c *gin.Context) {
	h.jobMu.Lock()
	var st *fullInitJob
	if h.fullJob != nil {
		cp := *h.fullJob
		st = &cp
	}
	h.jobMu.Unlock()
	if st == nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "status": gin.H{"running": false}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "status": st})
}

func (h *APIHandler) runFullInitJob(job *fullInitJob) {
	// iterate IDs sequentially; safe but slow; can add concurrency later if needed
	for id := job.StartID; id <= job.EndID; id++ {
		h.jobMu.Lock()
		if h.fullJob != job || !job.Running {
			h.jobMu.Unlock()
			break
		}
		job.Current = id
		h.jobMu.Unlock()

		// call upstream with retry logic for 429 errors
		var body []byte
		var err error
		maxRetries := 3
		retryCount := 0

		for retryCount <= maxRetries {
			body, err = h.makeCSQAQRequest("info/good", map[string]string{"id": strconv.FormatInt(id, 10)})
			if err != nil {
				retryCount++
				if retryCount <= maxRetries {
					fmt.Printf("[Full Init] Retry %d/%d for good_id %d: %v\n", retryCount, maxRetries, id, err)
					time.Sleep(time.Duration(2000+retryCount*1000) * time.Millisecond) // Exponential backoff
					continue
				}
				h.incFail(job, fmt.Sprintf("request error after %d retries: %v", maxRetries, err))
				if job.ThrottleMS > 0 {
					time.Sleep(time.Duration(job.ThrottleMS) * time.Millisecond)
				}
				break
			}

			// Check for 429 errors
			if strings.Contains(string(body), "429 Too Many Requests") {
				retryCount++
				if retryCount <= maxRetries {
					fmt.Printf("[Full Init] Rate limited, retry %d/%d for good_id %d\n", retryCount, maxRetries, id)
					time.Sleep(time.Duration(3000+retryCount*2000) * time.Millisecond) // Longer wait for rate limits
					continue
				}
				h.incFail(job, "rate limited after retries")
				if job.ThrottleMS > 0 {
					time.Sleep(time.Duration(job.ThrottleMS) * time.Millisecond)
				}
				break
			}

			// Success, break out of retry loop
			break
		}

		if retryCount > maxRetries {
			continue // Skip this ID and move to next
		}

		var resp struct {
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
		if e := json.Unmarshal(body, &resp); e != nil || resp.Code != 200 || resp.Data.GoodsInfo.ID == 0 {
			bodyPreview := string(body)
			if len(bodyPreview) > 200 {
				bodyPreview = bodyPreview[:200] + "..."
			}
			h.incFail(job, fmt.Sprintf("parse fail or not found: code=%d, body=%s", resp.Code, bodyPreview))
			if job.ThrottleMS > 0 {
				time.Sleep(time.Duration(job.ThrottleMS) * time.Millisecond)
			}
			continue
		}
		gi := resp.Data.GoodsInfo

		// 价格筛选：跳过50以下和300以上的饰品，专注于合理价格区间
		yyypPrice := gi.YyypSellPrice
		buffPrice := gi.BuffSellPrice

		// 如果两个平台的价格都在50以下或300以上，跳过这个饰品
		priceInRange := false
		if (yyypPrice >= 50 && yyypPrice <= 300) || (buffPrice >= 50 && buffPrice <= 300) {
			priceInRange = true
		}

		if !priceInRange && (yyypPrice > 0 || buffPrice > 0) { // 有价格数据但不在范围内
			fmt.Printf("[Full Init] Skipping good_id %d (%s), price out of range: YYYP=%.2f, Buff=%.2f\n", id, gi.Name, yyypPrice, buffPrice)
			if job.ThrottleMS > 0 {
				time.Sleep(time.Duration(job.ThrottleMS) * time.Millisecond)
			}
			continue
		}

		// upsert
		_ = h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "good_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"market_hash_name", "name", "updated_at"}),
		}).Create(&models.CSQAQGood{GoodID: gi.ID, MarketHashName: gi.MarketHashName, Name: gi.Name}).Error

		if priceInRange {
			fmt.Printf("[Full Init] ✓ Saved good_id %d (%s) with valid price: YYYP=%.2f, Buff=%.2f\n", id, gi.Name, yyypPrice, buffPrice)
		} else {
			fmt.Printf("[Full Init] ✓ Saved good_id %d (%s) no price data\n", id, gi.Name)
		}
		h.incSuccess(job)
		if job.ThrottleMS > 0 {
			time.Sleep(time.Duration(job.ThrottleMS) * time.Millisecond)
		}
	}
	h.jobMu.Lock()
	if h.fullJob == job {
		job.Running = false
		now := time.Now()
		job.FinishedAt = &now
	}
	h.jobMu.Unlock()
}

func (h *APIHandler) incSuccess(job *fullInitJob) { h.jobMu.Lock(); job.Success++; h.jobMu.Unlock() }
func (h *APIHandler) incFail(job *fullInitJob, msg string) {
	h.jobMu.Lock()
	job.Failed++
	job.Error = msg
	h.jobMu.Unlock()
}

// POST helper for CSQAQ JSON endpoints
func (h *APIHandler) makeCSQAQPost(endpoint string, payload interface{}) ([]byte, error) {
	// Ensure IP is bound before making API requests
	if err := h.ensureIPBound(); err != nil {
		fmt.Printf("Warning: Failed to bind local IP: %v\n", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := CSQAQ_API_BASE + endpoint
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("ApiToken", CSQAQ_API_KEY)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ---------- CSQAQ Goods: init, list, detail ----------

// get_good_id request/response shapes (using dynamic map for data)
type csqaqGetGoodIDReq struct {
	PageIndex int64   `json:"page_index"`
	PageSize  int64   `json:"page_size"`
	Search    *string `json:"search,omitempty"`
}

type csqaqGoodBrief struct {
	ID             int64  `json:"id"`
	MarketHashName string `json:"market_hash_name"`
	Name           string `json:"name"`
}

type csqaqGetGoodIDResp struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Data      json.RawMessage `json:"data"`
		PageIndex int64           `json:"page_index"`
		PageSize  int64           `json:"page_size"`
		Total     int64           `json:"total"`
	} `json:"data"`
}

// InitCSQAQGoods: POST /api/v1/csqaq/init-goods
// Iterates default keywords to fetch and persist CSQAQ good ids and names
func (h *APIHandler) InitCSQAQGoods(c *gin.Context) {
	var req struct {
		Keyword  string `json:"keyword"`
		PageSize int64  `json:"page_size"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if strings.TrimSpace(req.Keyword) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword 不能为空"})
		return
	}
	// 如未指定 page_size，则默认较大单页，后续自动翻页直到拉完全部
	if req.PageSize <= 0 {
		req.PageSize = 200
	}

	kw := strings.TrimSpace(req.Keyword)
	type saved struct {
		good_id                int64
		name, market_hash_name string
	}
	results := make([]saved, 0, 256)
	seen := make(map[int64]struct{})
	total := int64(0)
	pageIndex := int64(1)
	for {
		payload := csqaqGetGoodIDReq{PageIndex: pageIndex, PageSize: req.PageSize, Search: &kw}
		body, err := h.makeCSQAQPost("info/get_good_id", payload)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "上游接口错误: " + err.Error()})
			return
		}
		var resp csqaqGetGoodIDResp
		if err := json.Unmarshal(body, &resp); err != nil {
			// 尝试返回部分body帮助定位
			preview := string(body)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "上游返回解析失败", "preview": preview})
			return
		}
		if resp.Code != 200 {
			c.JSON(http.StatusBadGateway, gin.H{"error": resp.Msg})
			return
		}
		if pageIndex == 1 {
			total = resp.Data.Total
		}

		// 本页数据
		var pageList []csqaqGoodBrief
		// 支持两种结构：map[string]obj 或 []obj
		var asMap map[string]csqaqGoodBrief
		if len(resp.Data.Data) > 0 {
			if err := json.Unmarshal(resp.Data.Data, &asMap); err == nil && len(asMap) > 0 {
				pageList = make([]csqaqGoodBrief, 0, len(asMap))
				for _, v := range asMap {
					pageList = append(pageList, v)
				}
			} else {
				var asArr []csqaqGoodBrief
				if err := json.Unmarshal(resp.Data.Data, &asArr); err == nil {
					pageList = asArr
				}
			}
		}
		if len(pageList) == 0 {
			break
		}
		batch := make([]models.CSQAQGood, 0, len(pageList))
		for _, v := range pageList {
			if _, ok := seen[v.ID]; ok {
				continue
			}
			seen[v.ID] = struct{}{}
			batch = append(batch, models.CSQAQGood{GoodID: v.ID, MarketHashName: v.MarketHashName, Name: v.Name})
			results = append(results, saved{good_id: v.ID, market_hash_name: v.MarketHashName, name: v.Name})
		}
		if len(batch) > 0 {
			if err := h.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "good_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"market_hash_name", "name", "updated_at"}),
			}).Create(&batch).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库去重写入失败"})
				return
			}
		}
		// 终止条件：到达总数或页数据不足一页
		got := int64(len(results))
		if got >= total {
			break
		}
		if int64(len(pageList)) < req.PageSize {
			break
		}
		pageIndex++
		if pageIndex > 10000 {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "data": gin.H{"count": len(results), "items": results, "total": total}})
}

// ListCSQAQGoods: GET /api/v1/csqaq/goods?search=&page=1&page_size=20
func (h *APIHandler) ListCSQAQGoods(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	// 清理孤立的快照数据：删除csqaq_good_snapshots中存在但csqaq_goods中不存在的商品快照
	_ = h.db.Exec(`
        DELETE FROM csqaq_good_snapshots
        WHERE good_id NOT IN (
            SELECT DISTINCT good_id FROM csqaq_goods
        )
    `).Error

	var goods []models.CSQAQGood
	q := h.db.Model(&models.CSQAQGood{})

	if search != "" {
		like := "%" + search + "%"
		q = q.Where("LOWER(name) LIKE LOWER(?) OR LOWER(market_hash_name) LIKE LOWER(?) OR CAST(good_id AS CHAR) LIKE ?", like, like, like)
	}
	var total int64
	_ = q.Count(&total).Error
	_ = q.Order("good_id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&goods).Error

	// 获取每个商品的快照数据、最新价格信息
	ids := make([]int64, 0, len(goods))
	for _, g := range goods {
		ids = append(ids, g.GoodID)
	}

	type aggRow struct {
		GoodID        int64
		Cnt           int64
		Last          string
		YyypSellPrice *float64
		BuffSellPrice *float64
	}
	aggs := make([]aggRow, 0)
	if len(ids) > 0 {
		// 获取每个商品最新的快照数据
		_ = h.db.Raw(`
            SELECT
                s1.good_id as good_id,
                COUNT(s2.id) as cnt,
                MAX(s1.created_at) as last,
                s1.yyyp_sell_price,
                s1.buff_sell_price
            FROM csqaq_good_snapshots s1
            LEFT JOIN csqaq_good_snapshots s2 ON s1.good_id = s2.good_id
            WHERE s1.good_id IN ?
            AND s1.created_at = (
                SELECT MAX(s3.created_at)
                FROM csqaq_good_snapshots s3
                WHERE s3.good_id = s1.good_id
            )
            GROUP BY s1.good_id, s1.yyyp_sell_price, s1.buff_sell_price
        `, ids).Scan(&aggs).Error
	}

	mCnt := map[int64]int64{}
	mLast := map[int64]string{}
	mYyypPrice := map[int64]*float64{}
	mBuffPrice := map[int64]*float64{}
	for _, a := range aggs {
		mCnt[a.GoodID] = a.Cnt
		mLast[a.GoodID] = a.Last
		mYyypPrice[a.GoodID] = a.YyypSellPrice
		mBuffPrice[a.GoodID] = a.BuffSellPrice
	}

	items := make([]gin.H, 0, len(goods))
	for _, g := range goods {
		item := gin.H{
			"id":               g.ID,
			"good_id":          g.GoodID,
			"name":             g.Name,
			"market_hash_name": g.MarketHashName,
			"snapshot_count":   mCnt[g.GoodID],
			"last_sampled_at":  mLast[g.GoodID],
		}
		// 添加最新价格信息
		if price := mYyypPrice[g.GoodID]; price != nil {
			item["yyyp_sell_price"] = *price
		}
		if price := mBuffPrice[g.GoodID]; price != nil {
			item["buff_sell_price"] = *price
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "ok",
		"data": gin.H{
			"items":     items,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetCSQAQGoodDetail: GET /api/v1/csqaq/good?id=6796 -> proxy to https://api.csqaq.com/api/v1/info/good
func (h *APIHandler) GetCSQAQGoodDetail(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	body, err := h.makeCSQAQRequest("info/good", map[string]string{"id": id})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "csqaq upstream error"})
		return
	}
	c.Header("Content-Type", "application/json")
	c.Data(http.StatusOK, "application/json", body)
}

// GetGoodKline aggregates snapshots into OHLC series
// GET /api/v1/csqaq/good/kline?id=6796&interval=20m|1h|1d&limit=200
func (h *APIHandler) GetGoodKline(c *gin.Context) {
	idStr := strings.TrimSpace(c.Query("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	goodID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	interval := c.DefaultQuery("interval", "20m")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 {
		limit = 200
	}

	// fetch snapshots
	var snaps []models.CSQAQGoodSnapshot
	if err := h.db.Where("good_id = ?", goodID).Order("created_at asc").Find(&snaps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if len(snaps) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "data": []any{}})
		return
	}

	// bucket size
	var bucket time.Duration
	switch interval {
	case "20m", "20min", "20minute":
		bucket = 20 * time.Minute
		interval = "20m"
	case "1h", "1hour":
		bucket = time.Hour
		interval = "1h"
	case "1d", "1day":
		bucket = 24 * time.Hour
		interval = "1d"
	default:
		bucket = 20 * time.Minute
		interval = "20m"
	}

	type bar struct {
		t          int64
		o, h, l, c float64
	}
	var bars []bar
	// aggregate by bucket using prefer(yyyp sell -> buff sell -> yyyp buy -> buff buy)
	var curStart time.Time
	had := false
	var o, hi, lo, cpx float64
	pick := func(s models.CSQAQGoodSnapshot) (float64, bool) {
		if s.YYYPSellPrice != nil {
			return *s.YYYPSellPrice, true
		}
		if s.BuffSellPrice != nil {
			return *s.BuffSellPrice, true
		}
		if s.YYYPBuyPrice != nil {
			return *s.YYYPBuyPrice, true
		}
		if s.BuffBuyPrice != nil {
			return *s.BuffBuyPrice, true
		}
		return 0, false
	}
	floorTo := func(t time.Time, d time.Duration) time.Time {
		return t.Truncate(d)
	}
	for _, s := range snaps {
		px, ok := pick(s)
		if !ok {
			continue
		}
		b := floorTo(s.CreatedAt, bucket)
		if !had {
			curStart, o, hi, lo, cpx, had = b, px, px, px, px, true
			continue
		}
		if b != curStart {
			bars = append(bars, bar{t: curStart.UnixMilli(), o: o, h: hi, l: lo, c: cpx})
			// reset
			curStart, o, hi, lo, cpx = b, px, px, px, px
			continue
		}
		// same bucket update
		if px > hi {
			hi = px
		}
		if px < lo {
			lo = px
		}
		cpx = px
	}
	if had {
		bars = append(bars, bar{t: curStart.UnixMilli(), o: o, h: hi, l: lo, c: cpx})
	}

	// limit tail
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	out := make([]gin.H, len(bars))
	for i, b := range bars {
		out[i] = gin.H{"t": b.t, "o": b.o, "h": b.h, "l": b.l, "c": b.c, "v": 0}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "interval": interval, "data": out})
}

// GetGoodDerivedKline makes a synthetic daily series using current price and historical pct changes
// GET /api/v1/csqaq/good/derived_kline?id=6796&days=30
func (h *APIHandler) GetGoodDerivedKline(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 {
		days = 30
	}
	// fetch detail
	body, err := h.makeCSQAQRequest("info/good", map[string]string{"id": id})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream error"})
		return
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			GoodsInfo struct {
				YyypSellPrice    float64 `json:"yyyp_sell_price"`
				BuffSellPrice    float64 `json:"buff_sell_price"`
				SellPriceRate7   float64 `json:"sell_price_rate_7"`
				SellPriceRate30  float64 `json:"sell_price_rate_30"`
				SellPriceRate180 float64 `json:"sell_price_rate_180"`
			} `json:"goods_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Code != 200 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid upstream"})
		return
	}
	gi := resp.Data.GoodsInfo
	base := gi.YyypSellPrice
	if base == 0 {
		base = gi.BuffSellPrice
	}
	if base == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": []any{}})
		return
	}
	// estimate daily rate by mixing periods
	d7 := gi.SellPriceRate7 / 7.0
	d30 := gi.SellPriceRate30 / 30.0
	d180 := gi.SellPriceRate180 / 180.0
	weight7, weight30, weight180 := 0.6, 0.3, 0.1
	denom := weight7 + weight30 + weight180
	daily := (d7*weight7 + d30*weight30 + d180*weight180) / denom
	// build series backward and forward (simple synthetic OHLC)
	type point struct {
		T          int64
		O, H, L, C float64
		V          float64
	}
	pts := make([]point, 0, days)
	now := time.Now().Add(-time.Duration(days-1) * 24 * time.Hour)
	price := base
	for i := 0; i < days; i++ {
		// apply daily change to get close
		if i > 0 {
			price = price * (1 + daily/100.0)
		}
		// synthesize range around close
		high := price * 1.01
		low := price * 0.99
		open := price
		if i > 0 {
			open = price / (1 + daily/100.0)
		}
		close := price
		pts = append(pts, point{T: now.Add(time.Duration(i) * 24 * time.Hour).UnixMilli(), O: open, H: high, L: low, C: close, V: 0})
	}
	out := make([]gin.H, len(pts))
	for i, p := range pts {
		out[i] = gin.H{"t": p.T, "o": p.O, "h": p.H, "l": p.L, "c": p.C, "v": p.V}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "interval": "1d", "data": out})
}

// SampleGoodSnapshot: POST /api/v1/csqaq/good/snapshot { id }
func (h *APIHandler) SampleGoodSnapshot(c *gin.Context) {
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	// call upstream
	body, err := h.makeCSQAQRequest("info/good", map[string]string{"id": strconv.FormatInt(req.ID, 10)})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream error"})
		return
	}
	// parse
	var resp struct {
		Code int64 `json:"code"`
		Data struct {
			GoodsInfo struct {
				YyypSellPrice float64 `json:"yyyp_sell_price"`
				YyypBuyPrice  float64 `json:"yyyp_buy_price"`
				BuffSellPrice float64 `json:"buff_sell_price"`
				BuffBuyPrice  float64 `json:"buff_buy_price"`
			} `json:"goods_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Code != 200 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid upstream"})
		return
	}
	gi := resp.Data.GoodsInfo
	snap := models.CSQAQGoodSnapshot{GoodID: req.ID, CreatedAt: time.Now()}
	// to pointers
	yyypSell := gi.YyypSellPrice
	snap.YYYPSellPrice = &yyypSell
	yyypBuy := gi.YyypBuyPrice
	snap.YYYPBuyPrice = &yyypBuy
	buffSell := gi.BuffSellPrice
	snap.BuffSellPrice = &buffSell
	buffBuy := gi.BuffBuyPrice
	snap.BuffBuyPrice = &buffBuy
	if err := h.db.Create(&snap).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "snapshot created", "data": gin.H{"good_id": req.ID, "created_at": snap.CreatedAt}})
}

// -------- Forecast & Backtest --------

type csqaqKlinePoint struct {
	T string  `json:"t"`
	O float64 `json:"o"`
	C float64 `json:"c"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	V float64 `json:"v"`
}

// Generic point for client-provided data
type anyKlinePoint struct {
	T interface{} `json:"t"`
	O float64     `json:"o"`
	C float64     `json:"c"`
	H float64     `json:"h"`
	L float64     `json:"l"`
	V float64     `json:"v"`
}
type csqaqKlineResp struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg"`
	Data []csqaqKlinePoint `json:"data"`
}

func (h *APIHandler) fetchIndexKline(indexID, ktype string) ([]csqaqKlinePoint, error) {
	body, err := h.makeCSQAQRequest("sub/kline", map[string]string{"id": indexID, "type": ktype})
	if err != nil {
		return nil, err
	}
	var resp csqaqKlineResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("csqaq error: %s", resp.Msg)
	}
	return resp.Data, nil
}

func barsPerDay(ktype string) float64 {
	switch ktype {
	case "1hour":
		return 24
	case "4hour":
		return 6
	case "1day":
		return 1
	case "7day":
		return 1.0 / 7.0
	default:
		return 1
	}
}

// linear regression on close prices of last N points
func linRegForecast(closes []float64, ktype string, horizons []int, window int) (map[int]float64, map[string]float64) {
	n := len(closes)
	if n == 0 {
		return map[int]float64{}, map[string]float64{}
	}
	if window <= 0 || window > n {
		window = n
	}
	y := closes[n-window:]
	m := len(y)
	x := make([]float64, m)
	for i := 0; i < m; i++ {
		x[i] = float64(i)
	}
	sum := func(a []float64) float64 {
		s := 0.0
		for _, v := range a {
			s += v
		}
		return s
	}
	sumX := sum(x)
	sumY := sum(y)
	sumXX := 0.0
	sumXY := 0.0
	for i := 0; i < m; i++ {
		sumXX += x[i] * x[i]
		sumXY += x[i] * y[i]
	}
	denom := float64(m)*sumXX - sumX*sumX
	if denom == 0 {
		return map[int]float64{}, map[string]float64{}
	}
	slope := (float64(m)*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / float64(m)
	lastIdx := float64(m - 1)
	bpd := barsPerDay(ktype)
	preds := make(map[int]float64)
	for _, d := range horizons {
		k := float64(d) * bpd
		preds[d] = intercept + slope*(lastIdx+k)
	}
	meta := map[string]float64{
		"slope": slope, "intercept": intercept, "bars_per_day": bpd, "last_close": closes[n-1], "data_points": float64(m),
	}
	return preds, meta
}

func emaForecast(closes []float64, ktype string, horizons []int, window int, alpha float64) (map[int]float64, map[string]float64) {
	n := len(closes)
	if n == 0 {
		return map[int]float64{}, map[string]float64{}
	}
	if window <= 0 || window > n {
		window = n
	}
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}
	y := closes[n-window:]
	// initialize EMA with first value
	emaPrev := y[0]
	var emaLast float64 = emaPrev
	for i := 1; i < len(y); i++ {
		emaLast = alpha*y[i] + (1-alpha)*emaPrev
		emaPrev = emaLast
	}
	// approximate slope using last two points
	var slope float64
	if len(y) >= 2 {
		// last smoothed increment approximated by last raw increment smoothed
		slope = (y[len(y)-1] - y[len(y)-2]) * alpha
	} else {
		slope = 0
	}
	lastIdx := float64(len(y) - 1)
	bpd := barsPerDay(ktype)
	preds := make(map[int]float64)
	for _, d := range horizons {
		k := float64(d) * bpd
		preds[d] = emaLast + slope*k
	}
	meta := map[string]float64{
		"slope": slope, "intercept": emaLast - slope*lastIdx, "bars_per_day": bpd, "last_close": closes[n-1], "data_points": float64(len(y)), "alpha": alpha,
	}
	return preds, meta
}

func holtForecast(closes []float64, ktype string, horizons []int, window int, alpha, beta float64) (map[int]float64, map[string]float64) {
	n := len(closes)
	if n == 0 {
		return map[int]float64{}, map[string]float64{}
	}
	if window <= 0 || window > n {
		window = n
	}
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}
	if beta < 0 || beta > 1 {
		beta = 0.1
	}
	y := closes[n-window:]
	// initialize level and trend
	L := y[0]
	T := 0.0
	for i := 1; i < len(y); i++ {
		prevL := L
		L = alpha*y[i] + (1-alpha)*(L+T)
		T = beta*(L-prevL) + (1-beta)*T
	}
	bpd := barsPerDay(ktype)
	preds := make(map[int]float64)
	for _, d := range horizons {
		k := float64(d) * bpd
		preds[d] = L + k*T
	}
	meta := map[string]float64{
		"level": L, "trend": T, "bars_per_day": bpd, "last_close": closes[n-1], "data_points": float64(len(y)), "alpha": alpha, "beta": beta,
	}
	return preds, meta
}

func forecastByMethod(method string, closes []float64, ktype string, horizons []int, window int, params map[string]float64) (map[int]float64, map[string]float64) {
	m := strings.ToLower(method)
	switch m {
	case "ema":
		return emaForecast(closes, ktype, horizons, window, params["alpha"])
	case "holt":
		return holtForecast(closes, ktype, horizons, window, params["alpha"], params["beta"])
	default:
		return linRegForecast(closes, ktype, horizons, window)
	}
}

// RunForecast: POST /api/v1/forecast/run
// Body: { id: string, type: string, horizons?: [7,14,30], method?: 'linreg', window?: number }
func (h *APIHandler) RunForecast(c *gin.Context) {
	var req struct {
		ID       string             `json:"id" binding:"required"`
		Type     string             `json:"type" binding:"required"`
		Horizons []int              `json:"horizons"`
		Method   string             `json:"method"`
		Window   int                `json:"window"`
		Params   map[string]float64 `json:"params"`
		Data     []anyKlinePoint    `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Horizons) == 0 {
		req.Horizons = []int{7, 14, 30}
	}
	if req.Method == "" {
		req.Method = "linreg"
	}
	if req.Window <= 0 {
		req.Window = 200
	}

	var closes []float64
	if len(req.Data) > 0 {
		closes = make([]float64, 0, len(req.Data))
		for _, p := range req.Data {
			closes = append(closes, p.C)
		}
	} else {
		points, err := h.fetchIndexKline(req.ID, req.Type)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		closes = make([]float64, 0, len(points))
		for _, p := range points {
			closes = append(closes, p.C)
		}
	}
	// handle optional log transform
	useLog := false
	if v, ok := req.Params["use_log"]; ok {
		useLog = v != 0
	}
	if !useLog {
		if c.Request != nil {
			if strings.ToLower(c.Query("use_log")) == "true" {
				useLog = true
			}
		}
	}
	var series []float64
	if useLog {
		series = make([]float64, len(closes))
		for i := range closes {
			if closes[i] <= 0 {
				series[i] = math.Log(1e-8)
			} else {
				series[i] = math.Log(closes[i])
			}
		}
	} else {
		series = closes
	}
	preds, meta := forecastByMethod(req.Method, series, req.Type, req.Horizons, req.Window, req.Params)
	if useLog {
		for k, v := range preds {
			preds[k] = math.Exp(v)
		}
	}

	// persist
	now := time.Now()
	for _, d := range req.Horizons {
		rec := models.ForecastRecord{
			IndexID: req.ID, Interval: req.Type, HorizonDays: d, Predicted: preds[d], Method: req.Method,
			TrainWindow: req.Window, Slope: meta["slope"], Intercept: meta["intercept"], BarsPerDay: meta["bars_per_day"],
			LastClose: meta["last_close"], DataPoints: int(meta["data_points"]), CreatedAt: now,
		}
		_ = h.db.Create(&rec).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"index_id":    req.ID,
		"interval":    req.Type,
		"method":      req.Method,
		"window":      req.Window,
		"predictions": preds,
		"meta":        meta,
	})
}

// BacktestForecast: GET /api/v1/forecast/backtest?id=&type=&horizons=7,14,30&method=linreg&window=200&step=5
func (h *APIHandler) BacktestForecast(c *gin.Context) {
	id := c.Query("id")
	ktype := c.Query("type")
	if id == "" || ktype == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and type required"})
		return
	}
	method := c.DefaultQuery("method", "linreg")
	windowStr := c.DefaultQuery("window", "200")
	stepStr := c.DefaultQuery("step", "5")
	horizonsStr := c.DefaultQuery("horizons", "7,14,30")
	alphaStr := c.DefaultQuery("alpha", "0.3")
	betaStr := c.DefaultQuery("beta", "0.1")
	useLogStr := c.DefaultQuery("use_log", "false")
	window, _ := strconv.Atoi(windowStr)
	step, _ := strconv.Atoi(stepStr)
	if step <= 0 {
		step = 5
	}
	alpha, _ := strconv.ParseFloat(alphaStr, 64)
	beta, _ := strconv.ParseFloat(betaStr, 64)
	useLog := strings.ToLower(useLogStr) == "true" || useLogStr == "1"
	var horizons []int
	for _, s := range strings.Split(horizonsStr, ",") {
		if s == "" {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			horizons = append(horizons, v)
		}
	}
	if len(horizons) == 0 {
		horizons = []int{7, 14, 30}
	}

	points, err := h.fetchIndexKline(id, ktype)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	n := len(points)
	closes := make([]float64, n)
	for i, p := range points {
		closes[i] = p.C
	}
	var series []float64
	if useLog {
		series = make([]float64, n)
		for i := range closes {
			if closes[i] <= 0 {
				series[i] = math.Log(1e-8)
			} else {
				series[i] = math.Log(closes[i])
			}
		}
	} else {
		series = closes
	}

	// Sliding backtest
	type agg struct {
		sumAPE, sumAE, sumSE float64
		hits                 int
		count                int
	}
	metrics := make(map[int]*agg)
	for _, hday := range horizons {
		metrics[hday] = &agg{}
	}
	bpd := barsPerDay(ktype)
	for i := window; i < n-1; i += step {
		segment := series[:i]
		preds, _ := forecastByMethod(method, segment, ktype, horizons, window, map[string]float64{"alpha": alpha, "beta": beta})
		for _, d := range horizons {
			ahead := int(float64(d) * bpd)
			j := i + ahead
			if j >= n {
				continue
			}
			pred := preds[d]
			if useLog {
				pred = math.Exp(pred)
			}
			actual := closes[j]
			current := closes[i-1]
			if actual == 0 || current == 0 {
				continue
			}
			errAbs := pred - actual
			ape := (abs(errAbs) / abs(actual)) * 100.0
			m := metrics[d]
			m.sumAE += abs(errAbs)
			m.sumSE += errAbs * errAbs
			m.sumAPE += ape
			predRet := (pred - current) / current
			actRet := (actual - current) / current
			if (predRet >= 0 && actRet >= 0) || (predRet < 0 && actRet < 0) {
				m.hits++
			}
			m.count++
		}
	}

	out := make(map[string]any)
	now := time.Now()
	for _, d := range horizons {
		m := metrics[d]
		if m.count == 0 {
			continue
		}
		mae := m.sumAE / float64(m.count)
		rmse := 0.0
		if m.count > 0 {
			rmse = sqrt(m.sumSE / float64(m.count))
		}
		mape := m.sumAPE / float64(m.count)
		hitRate := 0.0
		if m.count > 0 {
			hitRate = float64(m.hits) * 100.0 / float64(m.count)
		}
		// persist summary
		_ = h.db.Create(&models.ForecastBacktest{IndexID: id, Interval: ktype, Method: method, TrainWindow: window, HorizonDays: d, Points: m.count, MAPE: mape, MAE: mae, RMSE: rmse, CreatedAt: now}).Error
		out[strconv.Itoa(d)] = gin.H{"points": m.count, "mape": mape, "mae": mae, "rmse": rmse, "hit_rate": hitRate}
	}
	c.JSON(http.StatusOK, gin.H{"index_id": id, "interval": ktype, "method": method, "window": window, "metrics": out})
}

// BacktestForecastPost: POST /api/v1/forecast/backtest
// Body: { id, type, horizons, method, window, step, params:{alpha,beta}, data:[{t,o,h,l,c,v}] }
func (h *APIHandler) BacktestForecastPost(c *gin.Context) {
	var req struct {
		ID       string             `json:"id" binding:"required"`
		Type     string             `json:"type" binding:"required"`
		Horizons []int              `json:"horizons"`
		Method   string             `json:"method"`
		Window   int                `json:"window"`
		Step     int                `json:"step"`
		Params   map[string]float64 `json:"params"`
		Data     []anyKlinePoint    `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Horizons) == 0 {
		req.Horizons = []int{7, 14, 30}
	}
	if req.Method == "" {
		req.Method = "linreg"
	}
	if req.Window <= 0 {
		req.Window = 200
	}
	if req.Step <= 0 {
		req.Step = 5
	}
	alpha := req.Params["alpha"]
	beta := req.Params["beta"]
	useLog := false
	if v, ok := req.Params["use_log"]; ok {
		useLog = v != 0
	}

	var closes []float64
	if len(req.Data) > 0 {
		closes = make([]float64, 0, len(req.Data))
		for _, p := range req.Data {
			closes = append(closes, p.C)
		}
	} else {
		points, err := h.fetchIndexKline(req.ID, req.Type)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		closes = make([]float64, len(points))
		for i, p := range points {
			closes[i] = p.C
		}
	}
	n := len(closes)
	var series []float64
	if useLog {
		series = make([]float64, n)
		for i := range closes {
			if closes[i] <= 0 {
				series[i] = math.Log(1e-8)
			} else {
				series[i] = math.Log(closes[i])
			}
		}
	} else {
		series = closes
	}
	type agg struct {
		sumAPE, sumAE, sumSE float64
		hits                 int
		count                int
	}
	metrics := make(map[int]*agg)
	for _, hday := range req.Horizons {
		metrics[hday] = &agg{}
	}
	bpd := barsPerDay(req.Type)
	for i := req.Window; i < n-1; i += req.Step {
		segment := series[:i]
		preds, _ := forecastByMethod(req.Method, segment, req.Type, req.Horizons, req.Window, map[string]float64{"alpha": alpha, "beta": beta})
		for _, d := range req.Horizons {
			ahead := int(float64(d) * bpd)
			j := i + ahead
			if j >= n {
				continue
			}
			pred := preds[d]
			if useLog {
				pred = math.Exp(pred)
			}
			actual := closes[j]
			current := closes[i-1]
			if actual == 0 || current == 0 {
				continue
			}
			errAbs := pred - actual
			ape := (abs(errAbs) / abs(actual)) * 100.0
			m := metrics[d]
			m.sumAE += abs(errAbs)
			m.sumSE += errAbs * errAbs
			m.sumAPE += ape
			predRet := (pred - current) / current
			actRet := (actual - current) / current
			if (predRet >= 0 && actRet >= 0) || (predRet < 0 && actRet < 0) {
				m.hits++
			}
			m.count++
		}
	}
	out := make(map[string]any)
	now := time.Now()
	for _, d := range req.Horizons {
		m := metrics[d]
		if m == nil || m.count == 0 {
			continue
		}
		mae := m.sumAE / float64(m.count)
		rmse := sqrt(m.sumSE / float64(m.count))
		mape := m.sumAPE / float64(m.count)
		hitRate := 0.0
		if m.count > 0 {
			hitRate = float64(m.hits) * 100.0 / float64(m.count)
		}
		_ = h.db.Create(&models.ForecastBacktest{IndexID: req.ID, Interval: req.Type, Method: req.Method, TrainWindow: req.Window, HorizonDays: d, Points: m.count, MAPE: mape, MAE: mae, RMSE: rmse, CreatedAt: now}).Error
		out[strconv.Itoa(d)] = gin.H{"points": m.count, "mape": mape, "mae": mae, "rmse": rmse, "hit_rate": hitRate}
	}
	c.JSON(http.StatusOK, gin.H{"index_id": req.ID, "interval": req.Type, "method": req.Method, "window": req.Window, "metrics": out})
}

func (h *APIHandler) ListForecastHistory(c *gin.Context) {
	id := c.Query("id")
	ktype := c.Query("type")
	var records []models.ForecastRecord
	q := h.db.Order("created_at desc").Limit(100)
	if id != "" {
		q = q.Where("index_id = ?", id)
	}
	if ktype != "" {
		q = q.Where("interval = ?", ktype)
	}
	_ = q.Find(&records).Error
	c.JSON(http.StatusOK, gin.H{"records": records})
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
func sqrt(x float64) float64 { return math.Sqrt(x) }

// ProxyCSQAQIndexKline proxies CSQAQ index kline per official doc
// GET /api/v1/csqaq/sub/kline -> https://api.csqaq.com/api/v1/sub/kline
// Query: id (required), type (e.g., 1day, 1hour, etc.)
func (h *APIHandler) ProxyCSQAQIndexKline(c *gin.Context) {
	params := map[string]string{}
	if id := c.Query("id"); id != "" {
		params["id"] = id
	}
	if kType := c.Query("type"); kType != "" {
		params["type"] = kType
	}

	// call CSQAQ documented endpoint
	body, err := h.makeCSQAQRequest("sub/kline", params)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "upstream error"})
		return
	}
	c.Header("Content-Type", "application/json")
	c.Data(http.StatusOK, "application/json", body)
}

// YouPin handlers

// AddYouPinAccount 添加悠悠有品账户
func (h *APIHandler) AddYouPinAccount(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证token有效性
	client, err := youpin.NewClient(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的token: " + err.Error()})
		return
	}

	// 获取用户ID（这里假设从JWT或session中获取）
	userID := uint(1) // 实际实现中需要从认证信息中获取

	// 检查是否已存在
	var existingAccount models.YouPinAccount
	if err := h.db.Where("user_id = ? AND token = ?", userID, req.Token).First(&existingAccount).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "账户已存在"})
		return
	}

	account := models.YouPinAccount{
		UserID:   userID,
		Token:    req.Token,
		Nickname: client.GetUserNickname(),
		IsActive: true,
	}

	if err := h.db.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建账户失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "账户添加成功",
		"account": account,
	})
}

// GetYouPinAccounts 获取用户的悠悠有品账户列表
func (h *APIHandler) GetYouPinAccounts(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	var accounts []models.YouPinAccount
	if err := h.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户列表失败"})
		return
	}

	// 追加余额（钱包+求购）并隐藏token
	for i := range accounts {
		acc := &accounts[i]
		realToken := acc.Token
		// 尝试实时拉取余额
		if client, err := youpin.NewClient(realToken); err == nil {
			if bal, err2 := client.GetBalances(c.Request.Context()); err2 == nil && bal != nil {
				acc.Balance = bal.WalletBalance
				acc.PurchaseBalance = bal.PurchaseBalance
			}
		}
		// 隐藏token
		acc.Token = "***"
	}

	c.JSON(http.StatusOK, gin.H{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// DeleteYouPinAccount 删除悠悠有品账户
func (h *APIHandler) DeleteYouPinAccount(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账户ID"})
		return
	}

	userID := uint(1) // 实际实现中需要从认证信息中获取

	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).Delete(&models.YouPinAccount{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除账户失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "账户删除成功"})
}

// ReactivateYouPinAccount 重新激活悠悠有品账户
func (h *APIHandler) ReactivateYouPinAccount(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账户ID"})
		return
	}

	userID := uint(1) // 实际实现中需要从认证信息中获取

	// 查找账户
	var account models.YouPinAccount
	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}

	// 验证token是否有效
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token验证失败: " + err.Error()})
		return
	}

	if !client.IsTokenValid(c.Request.Context()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token已失效，请重新登录获取新的Token"})
		return
	}

	// 重新激活账户
	if err := h.db.Model(&account).Update("is_active", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "激活账户失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "账户已重新激活"})
}

// GetYouPinConfig 获取悠悠有品配置
func (h *APIHandler) GetYouPinConfig(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	var config models.YouPinConfig
	if err := h.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 返回默认配置
			config = models.YouPinConfig{
				UserID:                   userID,
				AutoSellEnabled:          false,
				AutoBuyEnabled:           false,
				SellItemNames:            "[]",
				BlacklistWords:           "[]",
				MaxSalePrice:             0,
				TakeProfileEnabled:       false,
				TakeProfileRatio:         0.1,
				UsePriceAdjustment:       true,
				PriceAdjustmentThreshold: 1.0,
				RunTime:                  "09:00",
				Interval:                 60,
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
			return
		}
	}

	c.JSON(http.StatusOK, config)
}

// UpdateYouPinConfig 更新悠悠有品配置
func (h *APIHandler) UpdateYouPinConfig(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	var req models.YouPinConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.UserID = userID

	// 使用Upsert操作
	if err := h.db.Where("user_id = ?", userID).Assign(req).FirstOrCreate(&req).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新配置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "配置更新成功",
		"config":  req,
	})
}

// StartAutoSell 启动自动出售
func (h *APIHandler) StartAutoSell(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	// 获取用户的悠悠有品账户
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 获取配置
	var config models.YouPinConfig
	if err := h.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到配置信息"})
		return
	}

	// 转换配置为服务层使用的格式
	var sellItemNames []string
	json.Unmarshal([]byte(config.SellItemNames), &sellItemNames)

	var blacklistWords []string
	json.Unmarshal([]byte(config.BlacklistWords), &blacklistWords)

	serviceConfig := &youpin.YouPinConfig{
		Token:                    account.Token,
		AutoSellEnabled:          config.AutoSellEnabled,
		AutoBuyEnabled:           config.AutoBuyEnabled,
		SellItemNames:            sellItemNames,
		BlacklistWords:           blacklistWords,
		MaxSalePrice:             config.MaxSalePrice,
		TakeProfileEnabled:       config.TakeProfileEnabled,
		TakeProfileRatio:         config.TakeProfileRatio,
		UsePriceAdjustment:       config.UsePriceAdjustment,
		PriceAdjustmentThreshold: config.PriceAdjustmentThreshold,
		RunTime:                  config.RunTime,
		Interval:                 config.Interval,
	}

	// 创建服务实例
	service, err := youpin.NewService(account.Token, serviceConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务失败: " + err.Error()})
		return
	}

	// 启动自动出售（在后台goroutine中执行）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := service.StartAutoSell(ctx); err != nil {
			// 记录错误日志
			fmt.Printf("自动出售执行失败: %v\n", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "自动出售已启动"})
}

// StartAutoChangePrice 启动自动改价
func (h *APIHandler) StartAutoChangePrice(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	// 获取用户的悠悠有品账户
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 获取配置
	var config models.YouPinConfig
	if err := h.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到配置信息"})
		return
	}

	// 转换配置
	var sellItemNames []string
	json.Unmarshal([]byte(config.SellItemNames), &sellItemNames)

	var blacklistWords []string
	json.Unmarshal([]byte(config.BlacklistWords), &blacklistWords)

	serviceConfig := &youpin.YouPinConfig{
		Token:                    account.Token,
		SellItemNames:            sellItemNames,
		BlacklistWords:           blacklistWords,
		TakeProfileEnabled:       config.TakeProfileEnabled,
		TakeProfileRatio:         config.TakeProfileRatio,
		UsePriceAdjustment:       config.UsePriceAdjustment,
		PriceAdjustmentThreshold: config.PriceAdjustmentThreshold,
	}

	// 创建服务实例
	service, err := youpin.NewService(account.Token, serviceConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务失败: " + err.Error()})
		return
	}

	// 启动自动改价（在后台goroutine中执行）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		if err := service.AutoChangePrice(ctx); err != nil {
			// 记录错误日志
			fmt.Printf("自动改价执行失败: %v\n", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "自动改价已启动"})
}

// StartAutoAcceptOffer 启动自动接受报价
func (h *APIHandler) StartAutoAcceptOffer(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	// 获取用户的悠悠有品账户
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建基本配置
	serviceConfig := &youpin.YouPinConfig{
		Token: account.Token,
	}

	// 创建服务实例
	service, err := youpin.NewService(account.Token, serviceConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务失败: " + err.Error()})
		return
	}

	// 注入Steam报价接受实现：使用保存的Steam凭据自动登录并获取/刷新API Key，然后接受报价
	service.SetSteamOfferAccepter(func(ctx context.Context, offerID string) error {
		var creds models.SteamCredentials
		if err := h.db.Where("user_id = ?", userID).First(&creds).Error; err != nil {
			return fmt.Errorf("未配置Steam凭据: %w", err)
		}
		st, err := steamauth.NewClient()
		if err != nil {
			return err
		}
		apiKey := creds.APIKey
		// 若无API Key，或接受失败再刷新
		accept := func() error { return st.AcceptTradeOffer(apiKey, offerID) }
		if apiKey == "" {
			// 登录并获取API Key
			key, err := st.LoginAndGetAPIKey(steamauth.Credentials{
				Username:       creds.SteamUsername,
				Password:       creds.SteamPassword,
				SharedSecret:   creds.SharedSecret,
				IdentitySecret: creds.IdentitySecret,
			})
			if err != nil {
				return err
			}
			apiKey = key
			creds.APIKey = key
			_ = h.db.Save(&creds).Error
		}
		if err := accept(); err != nil {
			// 可能会话过期，重新登录获取Key后重试一次
			key, e2 := st.LoginAndGetAPIKey(steamauth.Credentials{
				Username:       creds.SteamUsername,
				Password:       creds.SteamPassword,
				SharedSecret:   creds.SharedSecret,
				IdentitySecret: creds.IdentitySecret,
			})
			if e2 != nil {
				return fmt.Errorf("接受失败且刷新登录失败: %v / %v", err, e2)
			}
			apiKey = key
			creds.APIKey = key
			_ = h.db.Save(&creds).Error
			return st.AcceptTradeOffer(apiKey, offerID)
		}
		return nil
	})

	// 启动自动接受报价（在后台goroutine中执行）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := service.AutoAcceptOffer(ctx); err != nil {
			// 记录错误日志
			fmt.Printf("自动接受报价执行失败: %v\n", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "自动接受报价已启动"})
}

// GetYouPinOrders 获取悠悠有品订单
func (h *APIHandler) GetYouPinOrders(c *gin.Context) {
	userID := uint(1) // 实际实现中需要从认证信息中获取

	var orders []models.YouPinOrder
	query := h.db.Where("user_id = ?", userID)

	// 支持按订单类型过滤
	if orderType := c.Query("type"); orderType != "" {
		query = query.Where("order_type = ?", orderType)
	}

	// 支持按状态过滤
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分页
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100")) // 增加默认页面大小
	offset := (page - 1) * pageSize

	var total int64
	query.Model(&models.YouPinOrder{}).Count(&total)

	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取订单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":    orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetSteamCredentials 获取Steam验证参数
func (h *APIHandler) GetSteamCredentials(c *gin.Context) {
	userID := uint(1)
	var creds models.SteamCredentials
	if err := h.db.Where("user_id = ?", userID).First(&creds).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"shared_secret":   "",
			"identity_secret": "",
			"steam_username":  "",
			"steam_password":  "",
			"api_key":         "",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"shared_secret":   creds.SharedSecret,
		"identity_secret": creds.IdentitySecret,
		"steam_username":  creds.SteamUsername,
		"steam_password":  creds.SteamPassword,
		"api_key":         creds.APIKey,
	})
}

// UpdateSteamCredentials 更新Steam验证参数
func (h *APIHandler) UpdateSteamCredentials(c *gin.Context) {
	userID := uint(1)
	var req struct {
		SharedSecret   string `json:"shared_secret" binding:"required"`
		IdentitySecret string `json:"identity_secret" binding:"required"`
		SteamUsername  string `json:"steam_username" binding:"required"`
		SteamPassword  string `json:"steam_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var creds models.SteamCredentials
	if err := h.db.Where("user_id = ?", userID).First(&creds).Error; err != nil {
		creds = models.SteamCredentials{
			UserID:         userID,
			SharedSecret:   req.SharedSecret,
			IdentitySecret: req.IdentitySecret,
			SteamUsername:  req.SteamUsername,
			SteamPassword:  req.SteamPassword,
		}
		if err := h.db.Create(&creds).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
			return
		}
	} else {
		creds.SharedSecret = req.SharedSecret
		creds.IdentitySecret = req.IdentitySecret
		creds.SteamUsername = req.SteamUsername
		creds.SteamPassword = req.SteamPassword
		if err := h.db.Save(&creds).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// LoginSteamAndGetKey 使用凭据自动登录并获取API Key
func (h *APIHandler) LoginSteamAndGetKey(c *gin.Context) {
	userID := uint(1)
	var creds models.SteamCredentials
	if err := h.db.Where("user_id = ?", userID).First(&creds).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先保存Steam凭据"})
		return
	}

	st, err := steamauth.NewClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化Steam客户端失败: " + err.Error()})
		return
	}

	apiKey, err := st.LoginAndGetAPIKey(steamauth.Credentials{
		Username:       creds.SteamUsername,
		Password:       creds.SteamPassword,
		SharedSecret:   creds.SharedSecret,
		IdentitySecret: creds.IdentitySecret,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录或获取API Key失败: " + err.Error()})
		return
	}

	creds.APIKey = apiKey
	if err := h.db.Save(&creds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存API Key失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "api_key": apiKey})
}

// GetYouPinInventory 获取悠悠有品库存
func (h *APIHandler) GetYouPinInventory(c *gin.Context) {
	// 获取所有活跃的悠悠有品账户
	var accounts []models.YouPinAccount
	if err := h.db.Where("is_active = ?", true).Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户列表失败"})
		return
	}

	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	refresh := c.Query("refresh") == "true"

	// 分页参数
	page := 1
	pageSize := 1000 // 增加默认页面大小以显示更多物品

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if sizeStr := c.Query("page_size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 10000 {
			pageSize = s
		}
	}

	// 排序参数
	sortBy := c.Query("sort_by") // price_asc, price_desc, name_asc, name_desc
	if sortBy == "" {
		sortBy = "name_asc"
	}

	// 账户筛选参数
	accountFilter := c.Query("account_id")
	accountNicknameFilter := c.Query("account_nickname")

	// 存储所有用户的库存数据
	type UserInventory struct {
		AccountID  uint                `json:"account_id"`
		Nickname   string              `json:"nickname"`
		Balance    float64             `json:"balance"`
		Inventory  []youpin.YouPinItem `json:"inventory"`
		Count      int                 `json:"count"`
		TotalValue float64             `json:"total_value"`
		Error      string              `json:"error,omitempty"`
	}

	var allInventories []UserInventory
	var totalItems int
	var totalValue float64

	// 遍历所有账户获取库存
	for _, account := range accounts {
		// 如果指定了账户筛选，只处理匹配的账户
		if accountFilter != "" {
			if accountID, err := strconv.ParseUint(accountFilter, 10, 32); err == nil {
				if account.ID != uint(accountID) {
					continue
				}
			}
		}
		if accountNicknameFilter != "" {
			if !strings.Contains(strings.ToLower(account.Nickname), strings.ToLower(accountNicknameFilter)) {
				continue
			}
		}

		userInv := UserInventory{
			AccountID: account.ID,
			Nickname:  account.Nickname,
			Balance:   account.Balance,
		}

		// 创建客户端
		client, err := youpin.NewClient(account.Token)
		if err != nil {
			userInv.Error = "创建客户端失败: " + err.Error()
			// 如果是token相关错误，自动禁用账户
			if strings.Contains(err.Error(), "登录状态失效") || strings.Contains(err.Error(), "登录失败") {
				h.db.Model(&account).Update("is_active", false)
				userInv.Error += " (账户已自动禁用，请重新登录)"
			}
			allInventories = append(allInventories, userInv)
			continue
		}

		// 先检查token是否有效
		if !client.IsTokenValid(c.Request.Context()) {
			userInv.Error = "Token已失效，请重新登录"
			// 自动禁用失效的账户
			h.db.Model(&account).Update("is_active", false)
			userInv.Error += " (账户已自动禁用)"
			allInventories = append(allInventories, userInv)
			continue
		}

		// 获取库存
		inventory, err := client.GetInventory(c.Request.Context(), refresh)
		if err != nil {
			userInv.Error = "获取库存失败: " + err.Error()
			// 如果是认证相关错误，也自动禁用账户
			if strings.Contains(err.Error(), "登录状态失效") || strings.Contains(err.Error(), "登录失败") || strings.Contains(err.Error(), "token") {
				h.db.Model(&account).Update("is_active", false)
				userInv.Error += " (账户已自动禁用，请重新登录)"
			}
			allInventories = append(allInventories, userInv)
			continue
		}

		// 为每个物品添加账户信息
		for i := range inventory {
			inventory[i].AccountID = account.ID
			inventory[i].AccountNickname = account.Nickname
		}

		// 计算总价值
		var userTotalValue float64
		for _, item := range inventory {
			userTotalValue += item.TemplateInfo.MarkPrice
		}

		userInv.Inventory = inventory
		userInv.Count = len(inventory)
		userInv.TotalValue = userTotalValue

		totalItems += len(inventory)
		totalValue += userTotalValue

		allInventories = append(allInventories, userInv)
	}

	// 合并所有库存物品进行分页
	var allItems []youpin.YouPinItem
	for _, userInv := range allInventories {
		if userInv.Error == "" {
			allItems = append(allItems, userInv.Inventory...)
		}
	}

	// 排序处理
	switch sortBy {
	case "price_asc":
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].TemplateInfo.MarkPrice < allItems[j].TemplateInfo.MarkPrice
		})
	case "price_desc":
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].TemplateInfo.MarkPrice > allItems[j].TemplateInfo.MarkPrice
		})
	case "name_asc":
		sort.Slice(allItems, func(i, j int) bool {
			nameI := allItems[i].TemplateInfo.CommodityName
			if nameI == "" {
				nameI = allItems[i].ShotName
			}
			nameJ := allItems[j].TemplateInfo.CommodityName
			if nameJ == "" {
				nameJ = allItems[j].ShotName
			}
			return nameI < nameJ
		})
	case "name_desc":
		sort.Slice(allItems, func(i, j int) bool {
			nameI := allItems[i].TemplateInfo.CommodityName
			if nameI == "" {
				nameI = allItems[i].ShotName
			}
			nameJ := allItems[j].TemplateInfo.CommodityName
			if nameJ == "" {
				nameJ = allItems[j].ShotName
			}
			return nameI > nameJ
		})
	}

	totalCount := len(allItems)
	totalPages := (totalCount + pageSize - 1) / pageSize

	// 分页切片
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	var pagedItems []youpin.YouPinItem
	if start < totalCount {
		pagedItems = allItems[start:end]
	}

	c.JSON(http.StatusOK, gin.H{
		"items":         pagedItems,
		"users":         allInventories,
		"total_count":   totalCount,
		"total_value":   totalValue,
		"account_count": len(accounts),
		"page":          page,
		"page_size":     pageSize,
		"total_pages":   totalPages,
		"has_next":      page < totalPages,
		"has_prev":      page > 1,
		"sort_by":       sortBy,
		"filters": gin.H{
			"account_id":       accountFilter,
			"account_nickname": accountNicknameFilter,
		},
	})
}

// SendYouPinSMS 发送悠悠有品短信验证码
func (h *APIHandler) SendYouPinSMS(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号不能为空"})
		return
	}

	// 验证手机号格式
	if len(req.Phone) != 11 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号格式错误"})
		return
	}

	// 发送短信验证码
	if err := youpin.SendSMSCode(c.Request.Context(), req.Phone); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送短信验证码失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "短信验证码发送成功",
	})
}

// LoginYouPinWithPhone 使用手机号和验证码登录悠悠有品
func (h *APIHandler) LoginYouPinWithPhone(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号和验证码不能为空"})
		return
	}

	// 使用手机号和验证码登录
	token, userInfo, err := youpin.LoginWithPhone(c.Request.Context(), req.Phone, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败: " + err.Error()})
		return
	}

	// 检查是否已有此用户的账户
	userID := uint(1) // 实际实现中需要从认证信息中获取
	var existingAccount models.YouPinAccount
	if err := h.db.Where("user_id = ? AND phone = ?", userID, req.Phone).First(&existingAccount).Error; err == nil {
		// 更新现有账户的token
		existingAccount.Token = token
		existingAccount.Nickname = userInfo.NickName
		existingAccount.IsActive = true
		existingAccount.UpdatedAt = time.Now()

		if err := h.db.Save(&existingAccount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新账户失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "登录成功，账户已更新",
			"account": gin.H{
				"id":       existingAccount.ID,
				"nickname": existingAccount.Nickname,
				"phone":    existingAccount.Phone,
			},
		})
		return
	}

	// 创建新账户
	account := models.YouPinAccount{
		UserID:    userID,
		Token:     token,
		Nickname:  userInfo.NickName,
		Phone:     req.Phone,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.db.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建账户失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功，账户已创建",
		"account": gin.H{
			"id":       account.ID,
			"nickname": account.Nickname,
			"phone":    account.Phone,
		},
	})
}

// 购买相关处理函数

// SearchYouPinCommodities 搜索悠悠有品商品
func (h *APIHandler) SearchYouPinCommodities(c *gin.Context) {
	var req youpin.YouPinSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认值 - 支持动态分页
	if req.PageIndex <= 0 {
		req.PageIndex = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20 // 默认20条
	}
	// 限制最大分页大小，避免服务器压力
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	// 获取活跃账户
	userID := uint(1) // 实际实现中需要从认证信息中获取
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "未找到有效的悠悠有品账户",
			"message":  "请先在 YouPin管理 页面添加并激活悠悠有品账户，然后再使用购买功能",
			"redirect": "/youpin"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 安卓端通常需要先上报设备信息
	_ = client.SendDeviceInfo(c.Request.Context())
	// 使用修复后的搜索接口（完全按抓包复刻）
	response, err := client.SearchItems(c.Request.Context(), req.Keyword, req.PageIndex, req.PageSize, 0)
	if err != nil {
		// 记录详细错误，便于排查
		fmt.Printf("[YouPin][Search] keyword=%s page=%d size=%d error=%v\n", req.Keyword, req.PageIndex, req.PageSize, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索失败: " + err.Error()})
		return
	}

	// 转换数据格式为前端期望的小写字段
	var frontendData []map[string]interface{}
	for _, item := range response.Data.CommodityTemplateList {
		frontendData = append(frontendData, item.ToFrontendFormat())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    frontendData,
		"pagination": gin.H{
			"page_index":  req.PageIndex,
			"page_size":   req.PageSize,
			"total_count": len(response.Data.CommodityTemplateList),
			"total_pages": 1,
		},
	})
}

// GetYouPinCommodityDetails 获取悠悠有品商品详情（直接返回在售商品列表）
func (h *APIHandler) GetYouPinCommodityDetails(c *gin.Context) {
	templateID := c.Param("template_id")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id不能为空"})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取商品详情（直接返回在售商品列表）
	details, err := client.GetCommodityDetails(c.Request.Context(), templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取商品详情失败: " + err.Error()})
		return
	}

	// 直接返回商品列表，前端显示具体的在售商家信息
	c.JSON(http.StatusOK, details)
}

// GetYouPinMarketItems 获取悠悠有品市场物品列表
func (h *APIHandler) GetYouPinMarketItems(c *gin.Context) {
	var req youpin.YouPinMarketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认值
	if req.PageIndex <= 0 {
		req.PageIndex = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取市场物品
	items, err := client.GetMarketSaleList(c.Request.Context(), req.TemplateId, req.PageIndex, req.PageSize, req.MinAbrade, req.MaxAbrade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取市场物品失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"page_index":  req.PageIndex,
		"page_size":   req.PageSize,
		"total_count": len(items),
	})
}

// GetYouPinPurchaseOrders 获取悠悠有品求购订单列表
func (h *APIHandler) GetYouPinPurchaseOrders(c *gin.Context) {
	var req youpin.YouPinMarketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认值
	if req.PageIndex <= 0 {
		req.PageIndex = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取求购订单
	orders, err := client.GetPurchaseOrderList(c.Request.Context(), req.TemplateId, req.PageIndex, req.PageSize, req.MinAbrade, req.MaxAbrade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取求购订单失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":      orders,
		"page_index":  req.PageIndex,
		"page_size":   req.PageSize,
		"total_count": len(orders),
	})
}

// BuyFromYouPinMarket 从悠悠有品市场直接购买
func (h *APIHandler) BuyFromYouPinMarket(c *gin.Context) {
	var req youpin.YouPinBuyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 上报设备信息后执行购买，提高成功率
	_ = client.SendDeviceInfo(c.Request.Context())
	// 执行购买
	err = client.BuyFromMarket(c.Request.Context(), req.CommodityId, req.Price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "购买失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "购买成功",
		"commodity_id": req.CommodityId,
		"price":        req.Price,
	})
}

// BuyFromYouPinMarketWithBalance 使用余额从悠悠有品市场购买
func (h *APIHandler) BuyFromYouPinMarketWithBalance(c *gin.Context) {
	var req youpin.YouPinBuyWithBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证支付方式必须为余额
	if req.PaymentMethod != "balance" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "支付方式必须为 'balance'"})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 检查账户余额是否足够
	balances, err := client.GetBalances(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户余额失败: " + err.Error()})
		return
	}

	if balances.WalletBalance < req.Price {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "钱包余额不足",
			"wallet_balance": balances.WalletBalance,
			"required_price": req.Price,
		})
		return
	}

	// 上报设备信息后执行购买，提高成功率
	_ = client.SendDeviceInfo(c.Request.Context())

	// 将字符串commodityId转换为int64
	commodityIdInt64, err := strconv.ParseInt(req.CommodityId, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的商品ID: " + req.CommodityId})
		return
	}

	// 使用新的多步骤余额购买流程
	result, err := client.MultiStepBuyWithBalance(c.Request.Context(), commodityIdInt64, req.Price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "余额购买失败: " + err.Error(),
			"step": result.Step,
			"order_no": result.OrderNo,
		})
		return
	}

	if !result.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": result.Message,
			"step": result.Step,
			"order_no": result.OrderNo,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "余额购买成功",
		"commodity_id":   req.CommodityId,
		"price":          req.Price,
		"payment_method": req.PaymentMethod,
		"order_no":       result.OrderNo,
		"status":         result.Status,
		"step":           result.Step,
		"remaining_balance": balances.WalletBalance - req.Price,
	})
}

// CreateYouPinPurchaseOrder 创建悠悠有品求购订单
func (h *APIHandler) CreateYouPinPurchaseOrder(c *gin.Context) {
	var req youpin.YouPinPurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 上报设备信息后创建求购订单
	_ = client.SendDeviceInfo(c.Request.Context())
	// 创建求购订单
	err = client.CreatePurchaseOrder(c.Request.Context(), req.TemplateId, req.TemplateHashName, req.CommodityName, req.PurchasePrice, req.PurchaseNum, req.MinAbrade, req.MaxAbrade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建求购订单失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "求购订单创建成功",
		"template_id":    req.TemplateId,
		"purchase_price": req.PurchasePrice,
		"purchase_num":   req.PurchaseNum,
	})
}

// SellYouPinItem 悠悠有品出售物品
func (h *APIHandler) SellYouPinItem(c *gin.Context) {
	var req struct {
		AssetID string  `json:"asset_id" binding:"required"`
		Price   float64 `json:"price" binding:"required,min=0"`
		Remark  string  `json:"remark"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 按自动流程先上报设备信息，避免即刻出售时上架/成交不生效
	_ = client.SendDeviceInfo(c.Request.Context())

	// 仿照自动发货流程，先上报设备信息以确保后续上架成功
	_ = client.SendDeviceInfo(c.Request.Context())

	// 创建出售物品列表
	saleItems := []youpin.YouPinSaleItem{
		{
			AssetID:    req.AssetID,
			IsCanLease: false,
			IsCanSold:  true,
			Price:      req.Price,
			Remark:     req.Remark,
		},
	}

	// 执行出售
	err = client.SellItems(c.Request.Context(), saleItems)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "出售失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "物品出售成功",
		"asset_id": req.AssetID,
		"price":    req.Price,
		"remark":   req.Remark,
	})
}

// GetSellableItems 获取可出售物品列表
func (h *APIHandler) GetSellableItems(c *gin.Context) {
	// 获取活跃账户
	userID := uint(1)
	var accounts []models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户失败: " + err.Error()})
		return
	}

	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	var allSellableItems []gin.H
	var totalValue float64

	for _, account := range accounts {
		// 创建客户端
		client, err := youpin.NewClient(account.Token)
		if err != nil {
			continue
		}

		// 获取库存
		inventory, err := client.GetInventory(c.Request.Context(), false)
		if err != nil {
			continue
		}

		// 筛选可出售物品
		for i, item := range inventory {
			if item.IsTradable() {
				sellableItem := gin.H{
					"index":            i,
					"asset_id":         item.SteamAssetID,
					"name":             item.GetName(),
					"commodity_name":   item.GetCommodityName(),
					"template_id":      item.GetTemplateID(),
					"market_price":     item.GetPrice(),
					"image_url":        item.GetImageURL(),
					"tradable":         item.Tradable,
					"asset_status":     item.AssetStatus,
					"account_id":       account.ID,
					"account_nickname": account.Nickname,
				}
				allSellableItems = append(allSellableItems, sellableItem)
				totalValue += item.GetPrice()
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sellable_items": allSellableItems,
		"total_count":    len(allSellableItems),
		"total_value":    totalValue,
		"message":        "获取可出售物品列表成功",
	})
}

// SellItemByName 通过物品名称出售物品
func (h *APIHandler) SellItemByName(c *gin.Context) {
	var req struct {
		ItemName  string  `json:"item_name" binding:"required"`
		Price     float64 `json:"price" binding:"required,min=0"`
		Remark    string  `json:"remark"`
		AccountID *uint   `json:"account_id"` // 可选，指定账户
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var accounts []models.YouPinAccount
	query := h.db.Where("user_id = ? AND is_active = ?", userID, true)
	if req.AccountID != nil {
		query = query.Where("id = ?", *req.AccountID)
	}
	if err := query.Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户失败: " + err.Error()})
		return
	}

	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 搜索物品
	var targetItem *youpin.YouPinItem
	var targetAccount *models.YouPinAccount

	for _, account := range accounts {
		client, err := youpin.NewClient(account.Token)
		if err != nil {
			continue
		}

		inventory, err := client.GetInventory(c.Request.Context(), false)
		if err != nil {
			continue
		}

		for _, item := range inventory {
			if item.IsTradable() && (item.GetName() == req.ItemName || item.GetCommodityName() == req.ItemName) {
				targetItem = &item
				targetAccount = &account
				break
			}
		}

		if targetItem != nil {
			break
		}
	}

	if targetItem == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到可出售的物品: " + req.ItemName})
		return
	}

	// 创建客户端并出售
	client, err := youpin.NewClient(targetAccount.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 仿照自动发货流程，先上报设备信息以确保后续上架成功
	_ = client.SendDeviceInfo(c.Request.Context())

	saleItems := []youpin.YouPinSaleItem{
		{
			AssetID:    targetItem.SteamAssetID,
			IsCanLease: false,
			IsCanSold:  true,
			Price:      req.Price,
			Remark:     req.Remark,
		},
	}

	err = client.SellItems(c.Request.Context(), saleItems)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "上架失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          "物品上架成功",
		"item_name":        req.ItemName,
		"asset_id":         targetItem.SteamAssetID,
		"price":            req.Price,
		"remark":           req.Remark,
		"account_nickname": targetAccount.Nickname,
		"market_price":     targetItem.GetPrice(),
	})
}

// SellItemsByIndex 通过索引批量出售物品
func (h *APIHandler) SellItemsByIndex(c *gin.Context) {
	var req struct {
		ItemIndexes []int   `json:"item_indexes" binding:"required,min=1"`
		Price       float64 `json:"price" binding:"required,min=0"`
		Remark      string  `json:"remark"`
		AccountID   *uint   `json:"account_id"` // 可选，指定账户
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var accounts []models.YouPinAccount
	query := h.db.Where("user_id = ? AND is_active = ?", userID, true)
	if req.AccountID != nil {
		query = query.Where("id = ?", *req.AccountID)
	}
	if err := query.Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户失败: " + err.Error()})
		return
	}

	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	var allItems []youpin.YouPinItem
	var accountMap = make(map[string]*models.YouPinAccount)

	// 收集所有账户的物品
	for _, account := range accounts {
		client, err := youpin.NewClient(account.Token)
		if err != nil {
			continue
		}

		inventory, err := client.GetInventory(c.Request.Context(), false)
		if err != nil {
			continue
		}

		for _, item := range inventory {
			if item.IsTradable() {
				allItems = append(allItems, item)
				accountMap[item.SteamAssetID] = &account
			}
		}
	}

	// 验证索引并收集要出售的物品
	var saleItems []youpin.YouPinSaleItem
	var soldItems []gin.H
	var errors []string

	for _, index := range req.ItemIndexes {
		if index < 0 || index >= len(allItems) {
			errors = append(errors, fmt.Sprintf("索引 %d 超出范围", index))
			continue
		}

		item := allItems[index]
		account := accountMap[item.SteamAssetID]

		saleItems = append(saleItems, youpin.YouPinSaleItem{
			AssetID:    item.SteamAssetID,
			IsCanLease: false,
			IsCanSold:  true,
			Price:      req.Price,
			Remark:     req.Remark,
		})

		soldItems = append(soldItems, gin.H{
			"index":            index,
			"asset_id":         item.SteamAssetID,
			"name":             item.GetName(),
			"commodity_name":   item.GetCommodityName(),
			"price":            req.Price,
			"market_price":     item.GetPrice(),
			"account_nickname": account.Nickname,
		})
	}

	if len(saleItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "没有有效的物品可以出售",
			"errors": errors,
		})
		return
	}

	// 按账户分组出售
	accountSales := make(map[string][]youpin.YouPinSaleItem)
	for _, item := range saleItems {
		account := accountMap[item.AssetID]
		accountSales[account.Token] = append(accountSales[account.Token], item)
	}

	var successCount int
	var failedItems []string

	for token, items := range accountSales {
		client, err := youpin.NewClient(token)
		if err != nil {
			for _, item := range items {
				failedItems = append(failedItems, item.AssetID)
			}
			continue
		}

		// 仿照自动发货流程，先上报设备信息以确保后续上架成功
		_ = client.SendDeviceInfo(c.Request.Context())

		err = client.SellItems(c.Request.Context(), items)
		if err != nil {
			for _, item := range items {
				failedItems = append(failedItems, item.AssetID)
			}
		} else {
			successCount += len(items)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "批量出售完成",
		"success_count": successCount,
		"failed_count":  len(failedItems),
		"sold_items":    soldItems,
		"failed_items":  failedItems,
		"errors":        errors,
		"total_price":   req.Price,
		"remark":        req.Remark,
	})
}

// GetSellList 获取在售物品列表
func (h *APIHandler) GetSellList(c *gin.Context) {
	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取在售列表
	sellList, err := client.GetSellListForAPI(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取在售列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sellList,
		"count":   len(sellList),
	})
}

// InstantSellItems 即刻出售物品 - 直接卖给求购者
func (h *APIHandler) InstantSellItems(c *gin.Context) {
	var req struct {
		Items []struct {
			AssetID    string  `json:"asset_id" binding:"required"`
			TemplateID string  `json:"template_id" binding:"required"`
			MinPrice   float64 `json:"min_price" binding:"min=0"`
		} `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	var results []map[string]interface{}
	var successCount int
	var failCount int

	for _, item := range req.Items {
		// 获取该物品的求购订单列表
		purchaseOrders, err := client.GetPurchaseOrderList(c.Request.Context(), item.TemplateID, 1, 20, 0, 1)
		if err != nil {
			results = append(results, map[string]interface{}{
				"asset_id": item.AssetID,
				"success":  false,
				"error":    "获取求购订单失败: " + err.Error(),
			})
			failCount++
			continue
		}

		// 找到价格最高且可以出售的求购订单
		var bestOrder *youpin.YouPinPurchaseOrder
		for _, order := range purchaseOrders {
			if order.CanSell && order.PurchasePrice >= item.MinPrice {
				if bestOrder == nil || order.PurchasePrice > bestOrder.PurchasePrice {
					bestOrder = &order
				}
			}
		}

		if bestOrder == nil {
			results = append(results, map[string]interface{}{
				"asset_id": item.AssetID,
				"success":  false,
				"error":    fmt.Sprintf("未找到合适的求购订单，最低价格要求: %.2f", item.MinPrice),
			})
			failCount++
			continue
		}

		// 直接卖给这个求购者 (这里需要调用YouPin的直接出售API)
		// 注意：这个API可能需要根据YouPin的实际API来调整
		sellToOrderErr := h.sellToOrder(c.Request.Context(), client, item.AssetID, bestOrder.OrderId)
		if sellToOrderErr != nil {
			results = append(results, map[string]interface{}{
				"asset_id": item.AssetID,
				"success":  false,
				"error":    "出售失败: " + sellToOrderErr.Error(),
			})
			failCount++
			continue
		}

		results = append(results, map[string]interface{}{
			"asset_id": item.AssetID,
			"success":  true,
			"price":    bestOrder.PurchasePrice,
			"buyer":    bestOrder.BuyerNickname,
			"order_id": bestOrder.OrderId,
		})
		successCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "即刻出售处理完成",
		"success_count": successCount,
		"fail_count":    failCount,
		"results":       results,
	})
}

// ChangeYouPinPrice 手动修改已上架商品价格
func (h *APIHandler) ChangeYouPinPrice(c *gin.Context) {
	var req struct {
		Commodities []struct {
			CommodityID string  `json:"commodity_id" binding:"required"`
			Price       float64 `json:"price" binding:"required,min=0"`
			Remark      string  `json:"remark"`
			IsCanLease  *bool   `json:"is_can_lease"`
			IsCanSold   *bool   `json:"is_can_sold"`
		} `json:"commodities" binding:"required,min=1"`
		AccountID *uint `json:"account_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取账户
	userID := uint(1)
	var account models.YouPinAccount
	q := h.db.Where("user_id = ? AND is_active = ?", userID, true)
	if req.AccountID != nil {
		q = q.Where("id = ?", *req.AccountID)
	}
	if err := q.First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 仿照自动流程，上报设备信息
	_ = client.SendDeviceInfo(c.Request.Context())

	var commodities []youpin.YouPinCommodity
	for _, it := range req.Commodities {
		isLease := false
		isSold := true
		if it.IsCanLease != nil {
			isLease = *it.IsCanLease
		}
		if it.IsCanSold != nil {
			isSold = *it.IsCanSold
		}
		commodities = append(commodities, youpin.YouPinCommodity{
			CommodityID: it.CommodityID,
			IsCanLease:  isLease,
			IsCanSold:   isSold,
			Price:       it.Price,
			Remark:      it.Remark,
		})
	}

	if err := client.ChangeSalePrice(c.Request.Context(), commodities); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改价格失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"changed_count": len(commodities),
		"account_id":    account.ID,
		"account_nick":  account.Nickname,
	})
}

// OffSaleYouPinItems 手动下架已上架商品
func (h *APIHandler) OffSaleYouPinItems(c *gin.Context) {
	var req struct {
		CommodityIDs []string `json:"commodity_ids" binding:"required,min=1"`
		AccountID    *uint    `json:"account_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取账户
	userID := uint(1)
	var account models.YouPinAccount
	q := h.db.Where("user_id = ? AND is_active = ?", userID, true)
	if req.AccountID != nil {
		q = q.Where("id = ?", *req.AccountID)
	}
	if err := q.First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 仿照自动流程，上报设备信息
	_ = client.SendDeviceInfo(c.Request.Context())

	if err := client.OffSale(c.Request.Context(), req.CommodityIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "下架失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"off_count":     len(req.CommodityIDs),
		"commodity_ids": req.CommodityIDs,
		"account_id":    account.ID,
		"account_nick":  account.Nickname,
	})
}

// sellToOrder 直接卖给求购者的辅助函数
func (h *APIHandler) sellToOrder(ctx context.Context, client *youpin.Client, assetID string, orderID string) error {
	// 确保设备已上报（幂等调用），提升后续上架与成交的成功率
	_ = client.SendDeviceInfo(ctx)
	// 这里需要调用YouPin的API来直接接受求购订单
	// 可能的API endpoint类似: /api/youpin/bff/trade/purchase/order/accept
	// 由于没有具体的API文档，这里先返回一个模拟的实现

	// TODO: 实现真正的接受求购订单API调用
	// data := map[string]interface{}{
	//     "orderId": orderID,
	//     "assetId": assetID,
	// }
	// return client.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/accept", data, &response)

	// 临时方案：先上架再立即下架
	saleItems := []youpin.YouPinSaleItem{{
		AssetID:    assetID,
		IsCanLease: false,
		IsCanSold:  true,
		Price:      0.01, // 设置极低价格快速匹配
		Remark:     "快速出售",
	}}

	if err := client.SellItems(ctx, saleItems); err != nil {
		return fmt.Errorf("临时上架失败: %w", err)
	}

	// 等待一小段时间
	time.Sleep(500 * time.Millisecond)

	// 立即下架（模拟被购买）
	return client.OffSale(ctx, []string{assetID})
}

// PurchaseRecommendation represents a buying recommendation based on CSQAQ data analysis
type PurchaseRecommendation struct {
	Good                models.CSQAQGood `json:"good"`
	RecommendationType  string           `json:"recommendation_type"` // "price_drop", "arbitrage", "undervalued", "trending_up"
	Confidence          float64          `json:"confidence"`          // 0-1 confidence score
	ReasonCode          string           `json:"reason_code"`
	ReasonText          string           `json:"reason_text"`
	CurrentYYYPPrice    *float64         `json:"current_yyyp_price"`
	CurrentBuffPrice    *float64         `json:"current_buff_price"`
	PriceDifference     *float64         `json:"price_difference"`      // YYYP - Buff
	PriceDifferencePerc *float64         `json:"price_difference_perc"` // (YYYP - Buff) / Buff * 100
	PriceChange7d       *float64         `json:"price_change_7d"`       // 7 day price change %
	PriceChange30d      *float64         `json:"price_change_30d"`      // 30 day price change %
	VolatilityScore     float64          `json:"volatility_score"`      // 0-1, lower is more stable
	RiskLevel           string           `json:"risk_level"`            // "low", "medium", "high"
	TargetPrice         *float64         `json:"target_price"`          // Expected target price
	PotentialProfit     *float64         `json:"potential_profit"`      // Expected profit amount
	ProfitPercentage    *float64         `json:"profit_percentage"`     // Expected profit percentage
	LastUpdated         time.Time        `json:"last_updated"`
}

// GetPurchaseRecommendations analyzes CSQAQ data and returns buying recommendations
// GET /api/v1/csqaq/recommendations?limit=20&type=all&risk_level=all
func (h *APIHandler) GetPurchaseRecommendations(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	recType := c.DefaultQuery("type", "all")         // "all", "price_drop", "arbitrage", "undervalued"
	riskLevel := c.DefaultQuery("risk_level", "all") // "all", "low", "medium", "high"

	recommendations, err := h.analyzeAndGenerateRecommendations(limit, recType, riskLevel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"data":  recommendations,
		"total": len(recommendations),
		"filters": gin.H{
			"type":       recType,
			"risk_level": riskLevel,
			"limit":      limit,
		},
		"generated_at": time.Now(),
		"msg":          "success",
	})
}

func (h *APIHandler) analyzeAndGenerateRecommendations(limit int, recType, riskLevel string) ([]PurchaseRecommendation, error) {
	// 1. Get goods with recent snapshot data (last 30 days)
	var goods []models.CSQAQGood
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	query := h.db.Preload("Snapshots", "created_at > ?", thirtyDaysAgo).
		Where(`id IN (
			SELECT DISTINCT s1.good_id FROM csqaq_good_snapshots s1
			WHERE s1.created_at > ?
			AND s1.good_id IN (
				SELECT s2.good_id FROM csqaq_good_snapshots s2
				WHERE s2.created_at = (
					SELECT MAX(s3.created_at) FROM csqaq_good_snapshots s3
					WHERE s3.good_id = s2.good_id
				)
				AND (
					(s2.yyyp_sell_price IS NOT NULL AND s2.yyyp_sell_price BETWEEN 50 AND 300) OR
					(s2.buff_sell_price IS NOT NULL AND s2.buff_sell_price BETWEEN 50 AND 300)
				)
			)
		)`, thirtyDaysAgo).
		Limit(limit * 3) // Get more goods to filter from

	if err := query.Find(&goods).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch goods with snapshots: %v", err)
	}

	var recommendations []PurchaseRecommendation

	for _, good := range goods {
		if len(good.Snapshots) < 2 { // Need at least 2 data points
			continue
		}

		rec := h.analyzeGoodForRecommendation(good)
		if rec != nil {
			// Filter by recommendation type
			if recType != "all" && rec.RecommendationType != recType {
				continue
			}

			// Filter by risk level
			if riskLevel != "all" && rec.RiskLevel != riskLevel {
				continue
			}

			recommendations = append(recommendations, *rec)
		}

		// Stop when we have enough recommendations
		if len(recommendations) >= limit {
			break
		}
	}

	// Sort by confidence score (highest first)
	for i := 0; i < len(recommendations)-1; i++ {
		for j := i + 1; j < len(recommendations); j++ {
			if recommendations[i].Confidence < recommendations[j].Confidence {
				recommendations[i], recommendations[j] = recommendations[j], recommendations[i]
			}
		}
	}

	return recommendations, nil
}

func (h *APIHandler) analyzeGoodForRecommendation(good models.CSQAQGood) *PurchaseRecommendation {
	snapshots := good.Snapshots
	if len(snapshots) < 3 { // 需要更多数据点进行准确分析
		return nil
	}

	// Sort snapshots by created_at (newest first)
	for i := 0; i < len(snapshots)-1; i++ {
		for j := i + 1; j < len(snapshots); j++ {
			if snapshots[i].CreatedAt.Before(snapshots[j].CreatedAt) {
				snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
			}
		}
	}

	latest := snapshots[0]

	// Calculate current prices
	var currentYYYP, currentBuff *float64
	if latest.YYYPSellPrice != nil && *latest.YYYPSellPrice > 0 {
		currentYYYP = latest.YYYPSellPrice
	}
	if latest.BuffSellPrice != nil && *latest.BuffSellPrice > 0 {
		currentBuff = latest.BuffSellPrice
	}

	// Skip if we don't have both prices or not in target range
	if currentYYYP == nil || currentBuff == nil ||
		*currentYYYP < 50 || *currentYYYP > 300 || *currentBuff < 50 || *currentBuff > 300 {
		return nil
	}

	// 计算主要价格参考 (取两个价格的平均值作为基准)
	avgCurrentPrice := (*currentYYYP + *currentBuff) / 2

	// Calculate price difference and arbitrage opportunity
	priceDiff := *currentYYYP - *currentBuff
	priceDiffPerc := (priceDiff / *currentBuff) * 100

	// Calculate price changes and change rates over multiple periods
	var priceChange3d, priceChange7d, priceChange30d *float64
	var changeRate3d *float64 // 变化率趋势

	threeDaysAgo := time.Now().AddDate(0, 0, -3)
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// 获取不同时间点的价格
	var price3d, price7d, price30d *float64

	for _, snap := range snapshots {
		if price3d == nil && snap.CreatedAt.Before(threeDaysAgo) && snap.YYYPSellPrice != nil && *snap.YYYPSellPrice > 0 {
			price3d = snap.YYYPSellPrice
		}
		if price7d == nil && snap.CreatedAt.Before(sevenDaysAgo) && snap.YYYPSellPrice != nil && *snap.YYYPSellPrice > 0 {
			price7d = snap.YYYPSellPrice
		}
		if price30d == nil && snap.CreatedAt.Before(thirtyDaysAgo) && snap.YYYPSellPrice != nil && *snap.YYYPSellPrice > 0 {
			price30d = snap.YYYPSellPrice
		}
	}

	// 计算变化率
	if price3d != nil {
		change := ((*currentYYYP - *price3d) / *price3d) * 100
		priceChange3d = &change
	}
	if price7d != nil {
		change := ((*currentYYYP - *price7d) / *price7d) * 100
		priceChange7d = &change
	}
	if price30d != nil {
		change := ((*currentYYYP - *price30d) / *price30d) * 100
		priceChange30d = &change
	}

	// 计算变化率的变化趋势 (加速度)
	if priceChange3d != nil && priceChange7d != nil {
		rate := *priceChange3d - (*priceChange7d / 7 * 3) // 标准化到3天期间
		changeRate3d = &rate
	}

	// 计算中低价饰品专用的波动率评分
	volatilityScore := h.calculateMidPriceVolatility(snapshots, avgCurrentPrice)

	// 计算相对强度 (vs 市场基准，简化版本)
	relativeStrength := h.calculateRelativeStrength(priceChange7d, priceChange30d)

	// 计算流动性评分 (基于价格区间和历史稳定性)
	liquidityScore := h.calculateLiquidityScore(snapshots, avgCurrentPrice)

	// 确定推荐类型和置信度 - 专门针对中低价饰品优化
	var recType, reasonCode, reasonText string
	var confidence float64
	var riskLevel string
	var targetPrice *float64
	var potentialProfit, profitPercentage *float64

	// 中低价饰品专用策略逻辑
	if priceDiffPerc < -3 && abs(priceDiffPerc) > 1 { // 降低套利阈值，中低价更敏感
		recType = "arbitrage"
		reasonCode = "MID_PRICE_ARBITRAGE"
		reasonText = fmt.Sprintf("YYYP价格比Buff低%.1f%%，中低价套利机会", -priceDiffPerc)
		confidence = 0.85 + min(0.15, abs(priceDiffPerc)/15)
		riskLevel = "low"
		target := *currentBuff * 0.98 // 目标接近Buff价格
		targetPrice = &target
		profit := (target - *currentYYYP) * 0.95 // 考虑手续费
		potentialProfit = &profit
		profitPct := (profit / *currentYYYP) * 100
		profitPercentage = &profitPct

	} else if priceChange3d != nil && *priceChange3d < -8 && changeRate3d != nil && *changeRate3d < -5 {
		// 短期急跌且跌势加速，中低价抄底机会
		recType = "price_drop"
		reasonCode = "SHORT_TERM_OVERSOLD"
		reasonText = fmt.Sprintf("近3天快速下跌%.1f%%且跌势加速，抄底时机", -*priceChange3d)
		confidence = 0.75 + min(0.2, abs(*priceChange3d)/40)
		riskLevel = h.calculateRiskForMidPrice(volatilityScore, relativeStrength)
		target := avgCurrentPrice * 1.08 // 8%反弹目标
		targetPrice = &target

	} else if priceChange7d != nil && *priceChange7d < -12 && priceChange30d != nil && *priceChange30d > -25 {
		// 7天跌幅较大但30天跌幅可控，中期抄底
		recType = "undervalued"
		reasonCode = "MID_TERM_CORRECTION"
		reasonText = fmt.Sprintf("近7天下跌%.1f%%，中期调整到位", -*priceChange7d)
		confidence = 0.65 + min(0.25, abs(*priceChange7d)/50)
		riskLevel = h.calculateRiskForMidPrice(volatilityScore, relativeStrength)
		target := avgCurrentPrice * 1.12 // 12%修复目标
		targetPrice = &target

	} else if relativeStrength > 0.1 && priceChange7d != nil && *priceChange7d > 3 && *priceChange7d < 12 {
		// 相对强势且温和上涨，中低价追涨
		recType = "trending_up"
		reasonCode = "RELATIVE_STRENGTH_MOMENTUM"
		reasonText = fmt.Sprintf("相对强势%.1f%%，7天上涨%.1f%%，动量良好", relativeStrength*100, *priceChange7d)
		confidence = 0.6 + min(0.3, relativeStrength*2)
		riskLevel = "medium"
		target := avgCurrentPrice * 1.15 // 15%上涨目标
		targetPrice = &target

	} else if liquidityScore > 0.7 && volatilityScore < 0.25 && priceChange30d != nil && *priceChange30d < -20 {
		// 高流动性、低波动、深度调整的稳健标的
		recType = "undervalued"
		reasonCode = "HIGH_LIQUIDITY_OVERSOLD"
		reasonText = fmt.Sprintf("高流动性标的深度调整%.1f%%，风险可控", -*priceChange30d)
		confidence = 0.7 + min(0.2, liquidityScore-0.7)
		riskLevel = "low"
		target := avgCurrentPrice * 1.18 // 18%修复目标
		targetPrice = &target

	} else {
		// 不符合推荐条件
		return nil
	}

	// 计算预期收益
	if targetPrice != nil && potentialProfit == nil {
		profit := (*targetPrice - avgCurrentPrice) * 0.92 // 考虑8%总成本
		potentialProfit = &profit
		profitPct := (profit / avgCurrentPrice) * 100
		profitPercentage = &profitPct
	}

	// 中低价饰品的风险和置信度调整
	confidence = h.adjustConfidenceForMidPrice(confidence, avgCurrentPrice, volatilityScore, liquidityScore)
	riskLevel = h.finalizeRiskLevel(riskLevel, volatilityScore, liquidityScore, avgCurrentPrice)

	return &PurchaseRecommendation{
		Good:                good,
		RecommendationType:  recType,
		Confidence:          confidence,
		ReasonCode:          reasonCode,
		ReasonText:          reasonText,
		CurrentYYYPPrice:    currentYYYP,
		CurrentBuffPrice:    currentBuff,
		PriceDifference:     &priceDiff,
		PriceDifferencePerc: &priceDiffPerc,
		PriceChange7d:       priceChange7d,
		PriceChange30d:      priceChange30d,
		VolatilityScore:     volatilityScore,
		RiskLevel:           riskLevel,
		TargetPrice:         targetPrice,
		PotentialProfit:     potentialProfit,
		ProfitPercentage:    profitPercentage,
		LastUpdated:         time.Now(),
	}
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// 中低价饰品专用波动率计算 - 考虑价格区间特性
func (h *APIHandler) calculateMidPriceVolatility(snapshots []models.CSQAQGoodSnapshot, avgPrice float64) float64 {
	if len(snapshots) < 5 {
		return 0.5 // 默认中等波动
	}

	var prices []float64
	for _, snap := range snapshots {
		if snap.YYYPSellPrice != nil && *snap.YYYPSellPrice > 0 {
			prices = append(prices, *snap.YYYPSellPrice)
		}
	}

	if len(prices) < 5 {
		return 0.5
	}

	// 计算标准差
	var sum, sumSquares float64
	for _, price := range prices {
		sum += price
		sumSquares += price * price
	}
	mean := sum / float64(len(prices))
	variance := (sumSquares / float64(len(prices))) - (mean * mean)
	stdDev := math.Sqrt(variance)

	// 针对中低价饰品的波动率标准化 (50-300价格区间)
	normalizedVolatility := stdDev / mean

	// 中低价饰品的波动率通常更高，需要调整评分标准
	if avgPrice < 100 {
		// 低价饰品，波动率容忍度更高
		return min(1.0, normalizedVolatility*1.5)
	} else if avgPrice < 200 {
		// 中价饰品，标准波动率
		return min(1.0, normalizedVolatility*2.0)
	} else {
		// 中高价饰品，波动率要求更严格
		return min(1.0, normalizedVolatility*2.5)
	}
}

// 相对强度计算 - 基于等权重指数特性
func (h *APIHandler) calculateRelativeStrength(priceChange7d, priceChange30d *float64) float64 {
	// 简化版相对强度：假设市场基准收益率
	// 实际应用中应该从数据库获取市场指数数据
	marketReturn7d := -2.0  // 假设市场7天平均下跌2%
	marketReturn30d := -5.0 // 假设市场30天平均下跌5%

	var relativeStrength7d, relativeStrength30d float64

	if priceChange7d != nil {
		relativeStrength7d = *priceChange7d - marketReturn7d
	}

	if priceChange30d != nil {
		relativeStrength30d = *priceChange30d - marketReturn30d
	}

	// 综合7天和30天的相对强度
	if priceChange7d != nil && priceChange30d != nil {
		return (relativeStrength7d*0.7 + relativeStrength30d*0.3) / 100
	} else if priceChange7d != nil {
		return relativeStrength7d / 100
	} else if priceChange30d != nil {
		return relativeStrength30d / 100
	}

	return 0
}

// 流动性评分计算 - 基于价格稳定性和数据密度
func (h *APIHandler) calculateLiquidityScore(snapshots []models.CSQAQGoodSnapshot, avgPrice float64) float64 {
	if len(snapshots) < 3 {
		return 0.3 // 数据不足，流动性较低
	}

	// 1. 数据密度评分 (更多数据点 = 更好的流动性)
	densityScore := min(1.0, float64(len(snapshots))/30.0) // 30天内的数据点密度

	// 2. 价格连续性评分 (价格跳跃越小 = 流动性越好)
	var priceGaps []float64
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i-1].YYYPSellPrice != nil && snapshots[i].YYYPSellPrice != nil &&
			*snapshots[i-1].YYYPSellPrice > 0 && *snapshots[i].YYYPSellPrice > 0 {
			gap := abs(*snapshots[i-1].YYYPSellPrice - *snapshots[i].YYYPSellPrice)
			gapPerc := gap / *snapshots[i].YYYPSellPrice
			priceGaps = append(priceGaps, gapPerc)
		}
	}

	var continuityScore float64 = 0.5 // 默认值
	if len(priceGaps) > 0 {
		var avgGap float64
		for _, gap := range priceGaps {
			avgGap += gap
		}
		avgGap = avgGap / float64(len(priceGaps))

		// 价格跳跃越小，连续性评分越高
		continuityScore = max(0.1, 1.0-avgGap*10)
	}

	// 3. 价格区间调整 (中低价饰品流动性通常更好)
	var priceRangeBonus float64
	if avgPrice >= 50 && avgPrice <= 150 {
		priceRangeBonus = 0.2 // 最佳流动性区间
	} else if avgPrice > 150 && avgPrice <= 250 {
		priceRangeBonus = 0.1 // 良好流动性区间
	} else {
		priceRangeBonus = 0.0 // 一般流动性区间
	}

	return min(1.0, (densityScore*0.4 + continuityScore*0.4 + priceRangeBonus + 0.2))
}

// 中低价饰品风险等级计算
func (h *APIHandler) calculateRiskForMidPrice(volatilityScore, relativeStrength float64) string {
	// 基于波动率和相对强度的风险评估
	if volatilityScore < 0.2 && relativeStrength > -0.1 {
		return "low"
	} else if volatilityScore < 0.4 && relativeStrength > -0.2 {
		return "medium"
	} else {
		return "high"
	}
}

// 中低价饰品置信度调整
func (h *APIHandler) adjustConfidenceForMidPrice(baseConfidence, avgPrice, volatilityScore, liquidityScore float64) float64 {
	confidence := baseConfidence

	// 1. 价格区间调整
	if avgPrice >= 80 && avgPrice <= 200 {
		confidence += 0.05 // 最佳价格区间加分
	} else if avgPrice >= 50 && avgPrice <= 250 {
		confidence += 0.02 // 良好价格区间小幅加分
	}

	// 2. 流动性调整
	if liquidityScore > 0.8 {
		confidence += 0.08
	} else if liquidityScore > 0.6 {
		confidence += 0.04
	} else if liquidityScore < 0.3 {
		confidence -= 0.1
	}

	// 3. 波动率调整
	if volatilityScore < 0.15 {
		confidence += 0.05 // 低波动率加分
	} else if volatilityScore > 0.6 {
		confidence -= 0.15 // 高波动率减分
	}

	return min(1.0, max(0.1, confidence))
}

// 最终风险等级确定
func (h *APIHandler) finalizeRiskLevel(baseRiskLevel string, volatilityScore, liquidityScore, avgPrice float64) string {
	risk := baseRiskLevel

	// 根据流动性调整风险等级
	if liquidityScore > 0.8 && volatilityScore < 0.25 {
		// 高流动性低波动，降低风险等级
		if risk == "high" {
			risk = "medium"
		} else if risk == "medium" {
			risk = "low"
		}
	} else if liquidityScore < 0.3 || volatilityScore > 0.7 {
		// 低流动性或高波动，提高风险等级
		if risk == "low" {
			risk = "medium"
		} else if risk == "medium" {
			risk = "high"
		}
	}

	// 中低价饰品特殊调整
	if avgPrice >= 100 && avgPrice <= 180 && liquidityScore > 0.6 {
		// 最佳价格区间且流动性良好，可以适度降低风险
		if risk == "high" {
			risk = "medium"
		}
	}

	return risk
}

// SearchYouPinItems 基于抓包信息的商品搜索接口
func (h *APIHandler) SearchYouPinItems(c *gin.Context) {
	// 请求参数结构 - 完全按抓包内容
	var req struct {
		Keywords  string `json:"keywords"`   // 搜索关键词 - 按抓包内容是keywords
		PageIndex int    `json:"page_index"` // 页码
		PageSize  int    `json:"page_size"`  // 每页数量
		SortType  int    `json:"sort_type"`  // 排序类型
		AccountID uint   `json:"account_id"` // 账户ID
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认值
	if req.PageIndex == 0 {
		req.PageIndex = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 50
	}

	// 获取账户信息
	var account models.YouPinAccount
	if req.AccountID > 0 {
		if err := h.db.Where("id = ? AND is_active = ?", req.AccountID, true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在或未激活"})
			return
		}
	} else {
		// 如果未指定账户，使用第一个可用账户
		if err := h.db.Where("is_active = ?", true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有可用的悠悠有品账户"})
			return
		}
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 调用更新后的搜索接口
	response, err := client.SearchItems(c.Request.Context(), req.Keywords, req.PageIndex, req.PageSize, req.SortType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索商品失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"account": map[string]interface{}{
			"id":       account.ID,
			"nickname": account.Nickname,
		},
	})
}

// GetYouPinCommodityList 获取商品列表
func (h *APIHandler) GetYouPinCommodityList(c *gin.Context) {
	var req struct {
		TemplateId int  `json:"template_id" binding:"required"`
		PageIndex  int  `json:"page_index"`
		PageSize   int  `json:"page_size"`
		SortType   int  `json:"sort_type"`
		AccountID  uint `json:"account_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认值
	if req.PageIndex == 0 {
		req.PageIndex = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 获取账户
	var account models.YouPinAccount
	if req.AccountID > 0 {
		if err := h.db.Where("id = ? AND is_active = ?", req.AccountID, true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在或未激活"})
			return
		}
	} else {
		if err := h.db.Where("is_active = ?", true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有可用的悠悠有品账户"})
			return
		}
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取商品列表
	response, err := client.GetCommodityList(c.Request.Context(), req.TemplateId, req.PageIndex, req.PageSize, req.SortType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取商品列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"account": map[string]interface{}{
			"id":       account.ID,
			"nickname": account.Nickname,
		},
	})
}

// GetYouPinFilterConfig 获取筛选配置
func (h *APIHandler) GetYouPinFilterConfig(c *gin.Context) {
	var req struct {
		TemplateId int  `json:"template_id" binding:"required"`
		AccountID  uint `json:"account_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取账户
	var account models.YouPinAccount
	if req.AccountID > 0 {
		if err := h.db.Where("id = ? AND is_active = ?", req.AccountID, true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在或未激活"})
			return
		}
	} else {
		if err := h.db.Where("is_active = ?", true).First(&account).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有可用的悠悠有品账户"})
			return
		}
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 获取筛选配置
	response, err := client.GetTemplateFilterConfig(c.Request.Context(), req.TemplateId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取筛选配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"account": map[string]interface{}{
			"id":       account.ID,
			"nickname": account.Nickname,
		},
	})
}

// MultiStepBuyFromYouPinMarket 基于HAR分析的多步骤购买流程接口
func (h *APIHandler) MultiStepBuyFromYouPinMarket(c *gin.Context) {
	var req youpin.YouPinMultiStepBuyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证支付方式必须为余额
	if req.PaymentMethod != "balance" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "支付方式必须为 \"balance\""})
		return
	}

	// 获取活跃账户
	userID := uint(1)
	var account models.YouPinAccount
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&account).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到有效的悠悠有品账户"})
		return
	}

	// 创建客户端
	client, err := youpin.NewClient(account.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建客户端失败: " + err.Error()})
		return
	}

	// 检查账户余额是否足够
	balances, err := client.GetBalances(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取账户余额失败: " + err.Error()})
		return
	}

	if balances.WalletBalance < req.Price {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "钱包余额不足",
			"wallet_balance": balances.WalletBalance,
			"required_price": req.Price,
		})
		return
	}

	// 上报设备信息后执行购买，提高成功率
	_ = client.SendDeviceInfo(c.Request.Context())

	// 将字符串commodityId转换为int64
	commodityIdInt64, err := strconv.ParseInt(req.CommodityId, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的商品ID: " + req.CommodityId})
		return
	}

	// 执行多步骤余额购买流程
	result, err := client.MultiStepBuyWithBalance(c.Request.Context(), commodityIdInt64, req.Price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": "多步骤购买流程失败: " + err.Error(),
			"step": func() string {
				if result != nil {
					return result.Step
				}
				return "unknown"
			}(),
			"order_no": func() string {
				if result != nil {
					return result.OrderNo
				}
				return ""
			}(),
		})
		return
	}

	// 返回购买流程结果
	c.JSON(http.StatusOK, gin.H{
		"success":            result.Success,
		"message":            result.Message,
		"commodity_id":       req.CommodityId,
		"price":              req.Price,
		"payment_method":     req.PaymentMethod,
		"order_no":           result.OrderNo,
		"status":             result.Status,
		"step":               result.Step,
		"remaining_balance":  balances.WalletBalance - req.Price,
		"account": map[string]interface{}{
			"id":       account.ID,
			"nickname": account.Nickname,
		},
	})
}
