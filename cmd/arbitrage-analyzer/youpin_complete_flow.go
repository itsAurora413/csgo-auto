package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// YouPinCompleteFlow 悠悠有品完整出售流程
type YouPinCompleteFlow struct {
	appKey     string
	steamID    string
	privateKey *rsa.PrivateKey
}

// SteamInventoryItem Steam库存项
type SteamInventoryItem struct {
	SteamID       string      `json:"steamId"`
	ItemAssetID   string      `json:"itemAssetId"`
	AssetDetail   AssetDetail `json:"assetDetail"`
	CommodityDetail interface{} `json:"commodityDetail"`
	MarketDetail  interface{} `json:"marketDetail"`
}

// AssetDetail 饰品详情
type AssetDetail struct {
	ItemAssetID      string `json:"itemAssetId"`
	TemplateID       int64  `json:"templateId"`
	TemplateHashName string `json:"templateHashName"`
	TemplateName     string `json:"templateName"`
	Marketable       int    `json:"marketable"`
	Tradable         int    `json:"tradable"`
	AssetStatus      int    `json:"assetStatus"`
}

// GetSteamInventoryRequest 获取库存请求
type GetSteamInventoryRequest struct {
	AppKey    string `json:"appKey"`
	Timestamp string `json:"timestamp"`
	Sign      string `json:"sign"`
	SteamID   string `json:"steamId"`
}

// OnShelfRequest 上架请求
type OnShelfRequest struct {
	AppKey      string                `json:"appKey"`
	Timestamp   string                `json:"timestamp"`
	Sign        string                `json:"sign"`
	SteamID     string                `json:"steamId"`
	RequestList []OnShelfRequestItem `json:"requestList"`
}

type OnShelfRequestItem struct {
	ItemAssetID string  `json:"itemAssetId"`
	Price       float64 `json:"price"`
}

// APIResponse 通用API响应
type APIResponse struct {
	Code      int         `json:"code"`
	Msg       string      `json:"msg"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// NewYouPinCompleteFlow 创建实例
func NewYouPinCompleteFlow(appKey, steamID, privateKeyPEM string) (*YouPinCompleteFlow, error) {
	// 解析私钥
	privateKeyBytes := []byte(privateKeyPEM)
	privateKeyObj, err := x509.ParsePKCS1PrivateKey(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %v", err)
	}

	return &YouPinCompleteFlow{
		appKey:     appKey,
		steamID:    steamID,
		privateKey: privateKeyObj,
	}, nil
}

// GenerateSign 生成签名
func (f *YouPinCompleteFlow) GenerateSign(data string) (string, error) {
	hash := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.privateKey, 0, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// GetSteamInventory 读取Steam库存
func (f *YouPinCompleteFlow) GetSteamInventory(ctx context.Context) ([]SteamInventoryItem, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	req := GetSteamInventoryRequest{
		AppKey:    f.appKey,
		Timestamp: timestamp,
		SteamID:   f.steamID,
	}

	// 生成签名
	dataToSign := fmt.Sprintf("appKey=%s&timestamp=%s&steamId=%s",
		req.AppKey, req.Timestamp, req.SteamID)
	sign, err := f.GenerateSign(dataToSign)
	if err != nil {
		return nil, fmt.Errorf("生成签名失败: %v", err)
	}
	req.Sign = sign

	// 发送请求
	url := "https://gw-openapi.youpin898.com/open/v1/api/getUserSteamInventoryData"

	jsonData, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("[库存查询] 响应内容: %s", string(body))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	// 解析数据
	dataBytes, _ := json.Marshal(apiResp.Data)
	var items []SteamInventoryItem
	if err := json.Unmarshal(dataBytes, &items); err != nil {
		log.Printf("[库存查询] 数据: %s", string(dataBytes))
		return nil, fmt.Errorf("解析库存失败: %v", err)
	}

	log.Printf("[库存查询] 成功获取 %d 个库存物品", len(items))
	return items, nil
}

// OnShelfCommodity 上架库存饰品
func (f *YouPinCompleteFlow) OnShelfCommodity(ctx context.Context, items []OnShelfRequestItem) error {
	if len(items) == 0 {
		return fmt.Errorf("上架列表为空")
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	req := OnShelfRequest{
		AppKey:      f.appKey,
		Timestamp:   timestamp,
		SteamID:     f.steamID,
		RequestList: items,
	}

	// 生成签名 - 注意顺序很重要
	dataToSign := fmt.Sprintf("appKey=%s&timestamp=%s&steamId=%s",
		req.AppKey, req.Timestamp, req.SteamID)
	sign, err := f.GenerateSign(dataToSign)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}
	req.Sign = sign

	// 发送请求
	url := "https://gw-openapi.youpin898.com/open/v1/api/onShelfCommodity"

	jsonData, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("[上架] 响应内容: %s", string(body))
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	log.Printf("[上架] 成功上架 %d 个物品", len(items))
	return nil
}

// OffShelfCommodity 下架商品（改用下架接口）
func (f *YouPinCompleteFlow) OffShelfCommodity(ctx context.Context, commodityIDs []int64) error {
	if len(commodityIDs) == 0 {
		return fmt.Errorf("下架列表为空")
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	type OffShelfReq struct {
		AppKey      string  `json:"appKey"`
		Timestamp   string  `json:"timestamp"`
		Sign        string  `json:"sign"`
		CommodityIDs []int64 `json:"commodityIds"`
	}

	req := OffShelfReq{
		AppKey:      f.appKey,
		Timestamp:   timestamp,
		CommodityIDs: commodityIDs,
	}

	dataToSign := fmt.Sprintf("appKey=%s&timestamp=%s",
		req.AppKey, req.Timestamp)
	sign, err := f.GenerateSign(dataToSign)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}
	req.Sign = sign

	url := "https://gw-openapi.youpin898.com/open/v1/api/offShelfCommodity"

	jsonData, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("[下架] 响应内容: %s", string(body))
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	log.Printf("[下架] 成功下架 %d 个商品", len(commodityIDs))
	return nil
}

// ChangeCommodityPrice 改价
func (f *YouPinCompleteFlow) ChangeCommodityPrice(ctx context.Context, changes map[int64]float64) error {
	if len(changes) == 0 {
		return fmt.Errorf("改价列表为空")
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	type ChangePriceItem struct {
		CommodityID int64   `json:"commodityId"`
		Price       float64 `json:"price"`
	}

	type ChangePriceReq struct {
		AppKey      string           `json:"appKey"`
		Timestamp   string           `json:"timestamp"`
		Sign        string           `json:"sign"`
		SteamID     string           `json:"steamId"`
		RequestList []ChangePriceItem `json:"requestList"`
	}

	var requestList []ChangePriceItem
	for cid, price := range changes {
		requestList = append(requestList, ChangePriceItem{
			CommodityID: cid,
			Price:       price,
		})
	}

	req := ChangePriceReq{
		AppKey:      f.appKey,
		Timestamp:   timestamp,
		SteamID:     f.steamID,
		RequestList: requestList,
	}

	dataToSign := fmt.Sprintf("appKey=%s&timestamp=%s&steamId=%s",
		req.AppKey, req.Timestamp, req.SteamID)
	sign, err := f.GenerateSign(dataToSign)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}
	req.Sign = sign

	url := "https://gw-openapi.youpin898.com/open/v1/api/commodityChangePrice"

	jsonData, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("[改价] 响应内容: %s", string(body))
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API错误: %s (code: %d)", apiResp.Msg, apiResp.Code)
	}

	log.Printf("[改价] 成功改价 %d 个商品", len(changes))
	return nil
}
