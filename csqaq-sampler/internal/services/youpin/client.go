package youpin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	_ "os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	BaseURL    = "https://api.youpin898.com"
	GameIDCSGO = "730"
)

// Client 悠悠有品API客户端
type Client struct {
	httpClient  *http.Client
	token       string
	deviceToken string
	deviceID    string
	userId      string
	nickname    string
	baseURL     string

	// 开放平台认证相关
	useOpenAPI bool       // 是否使用开放平台API
	rsaSigner  *RSASigner // RSA签名器（开放平台使用）
	openAPIURL string     // 开放平台API地址
}

// AccountBalances 账户余额信息
type AccountBalances struct {
	WalletBalance   float64
	PurchaseBalance float64
}

// GetBalances 获取账户余额（钱包余额/求购余额）
func (c *Client) GetBalances(ctx context.Context) (*AccountBalances, error) {
	// 使用正确的悠悠有品余额查询接口
	data := map[string]interface{}{
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Balance         string `json:"balance"`         // 钱包余额 (字符串)
			PurchaseBalance string `json:"purchaseBalance"` // 求购余额 (字符串)
			BalanceFroze    string `json:"balanceFroze"`    // 冻结余额 (字符串)
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/user/account/info", data, &response)
	if err != nil {
		// 如果新接口失败，使用旧接口作为备用
		return c.getBalancesLegacy(ctx)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取余额失败: %s", response.Msg)
	}

	// 将字符串转换为float64
	walletBalance, _ := strconv.ParseFloat(response.Data.Balance, 64)
	purchaseBalance, _ := strconv.ParseFloat(response.Data.PurchaseBalance, 64)

	return &AccountBalances{
		WalletBalance:   walletBalance,   // 钱包余额
		PurchaseBalance: purchaseBalance, // 求购余额
	}, nil
}

// getBalancesLegacy 备用的旧版余额查询方法
func (c *Client) getBalancesLegacy(ctx context.Context) (*AccountBalances, error) {
	// 适配不同网关，优先尝试bff资产接口，不同环境字段名可能略有差异
	var resp1 map[string]interface{}
	if err := c.makeRequest(ctx, "GET", "/api/youpin/bff/user/asset/info", map[string]interface{}{"Sessionid": c.deviceToken}, &resp1); err == nil {
		if code, ok := resp1["code"].(float64); ok && code == 0 {
			if data, ok := resp1["data"].(map[string]interface{}); ok {
				bal := &AccountBalances{}
				// 兼容多种字段命名
				extract := func(keys ...string) float64 {
					for _, k := range keys {
						if v, ok := data[k]; ok {
							switch vt := v.(type) {
							case float64:
								return vt
							case int:
								return float64(vt)
							case string:
								if f, err := strconv.ParseFloat(vt, 64); err == nil {
									return f
								}
							}
						}
					}
					return 0
				}
				bal.WalletBalance = extract("balance", "walletBalance", "availableBalance")
				bal.PurchaseBalance = extract("purchaseBalance", "buyBalance", "purchase_available")
				return bal, nil
			}
		}
	}

	// 退回到通用账户资产接口（安卓端）
	var resp2 struct {
		Code    int                    `json:"Code"`
		Message string                 `json:"Message"`
		Data    map[string]interface{} `json:"Data"`
	}
	if err := c.makeRequest(ctx, "GET", "/api/user/Account/asset", map[string]interface{}{"Sessionid": c.deviceToken}, &resp2); err == nil && resp2.Code == 0 {
		bal := &AccountBalances{}
		if v, ok := resp2.Data["Balance"]; ok {
			switch vt := v.(type) {
			case float64:
				bal.WalletBalance = vt
			case string:
				if f, err := strconv.ParseFloat(vt, 64); err == nil {
					bal.WalletBalance = f
				}
			}
		}
		if v, ok := resp2.Data["PurchaseBalance"]; ok {
			switch vt := v.(type) {
			case float64:
				bal.PurchaseBalance = vt
			case string:
				if f, err := strconv.ParseFloat(vt, 64); err == nil {
					bal.PurchaseBalance = f
				}
			}
		}
		return bal, nil
	}

	return nil, fmt.Errorf("无法获取账户余额")
}

// NewClient 创建新的悠悠有品客户端（使用Token认证）
// token: 用户的session token
// 注意：这是传统的Token认证方式，用于获取账户、搜索、购买等所有需要用户认证的操作
func NewClient(token string) (*Client, error) {
	return NewClientWithToken(token)
}

// NewClientWithToken 创建新的悠悠有品客户端（使用Token认证）
func NewClientWithToken(token string) (*Client, error) {
	deviceToken := "aNbW21QU7cUDAJB4bK22q1rk"

	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:       token,
		deviceToken: deviceToken,
		deviceID:    deviceToken, // 使用相同的Token作为设备ID
		baseURL:     BaseURL,
		useOpenAPI:  false, // 使用传统Token认证
	}

	// 获取用户信息
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userInfo, err := client.getUserInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("悠悠有品账号登录失败，请检查token是否正确: %w", err)
	}

	// 处理UserId的类型转换
	if userId, ok := userInfo.UserId.(string); ok {
		client.userId = userId
	} else if userIdNum, ok := userInfo.UserId.(float64); ok {
		client.userId = fmt.Sprintf("%.0f", userIdNum)
	} else {
		client.userId = fmt.Sprintf("%v", userInfo.UserId)
	}
	client.nickname = userInfo.NickName

	return client, nil
}

// NewClientWithOpenAPI 创建使用开放平台认证的客户端
// privateKeyBase64: Base64编码的PKCS8格式私钥
// appKey: 悠悠有品分配的AppKey
func NewClientWithOpenAPI(privateKeyBase64 string, appKey string) (*Client, error) {
	// 创建RSA签名器
	rsaSigner, err := NewRSASigner(privateKeyBase64, appKey)
	if err != nil {
		return nil, fmt.Errorf("创建RSA签名器失败: %w", err)
	}

	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		useOpenAPI: true,
		rsaSigner:  rsaSigner,
		baseURL:    BaseURL,
		openAPIURL: "https://gw-openapi.youpin898.com", // 开放平台网关地址
	}

	return client, nil
}

// NewClientWithTokenAndProxy 创建使用Token认证且支持代理的客户端
// token: 用户的session token
// proxyURL: 代理地址，格式: http://username:password@host:port 或 http://host:port
// timeout: 请求超时时间
func NewClientWithTokenAndProxy(token string, proxyURL string, timeout time.Duration) (*Client, error) {
	deviceToken := "aNbW21QU7cUDAJB4bK22q1rk"

	// 创建HTTP客户端，支持代理
	httpClient := &http.Client{
		Timeout: timeout,
	}

	// 配置代理
	if proxyURL != "" {
		proxyFunc := func(_ *http.Request) (*url.URL, error) {
			return url.Parse(proxyURL)
		}
		transport := &http.Transport{
			Proxy: proxyFunc,
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
		}
		httpClient.Transport = transport
	}

	client := &Client{
		httpClient:  httpClient,
		token:       token,
		deviceToken: deviceToken,
		deviceID:    deviceToken,
		baseURL:     BaseURL,
		useOpenAPI:  false,
	}

	// 获取用户信息验证token，使用带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	userInfo, err := client.getUserInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("悠悠有品账号登录失败，请检查token是否正确: %w", err)
	}

	// 处理UserId的类型转换
	if userId, ok := userInfo.UserId.(string); ok {
		client.userId = userId
	} else if userIdNum, ok := userInfo.UserId.(float64); ok {
		client.userId = fmt.Sprintf("%.0f", userIdNum)
	} else {
		client.userId = fmt.Sprintf("%v", userInfo.UserId)
	}
	client.nickname = userInfo.NickName

	return client, nil
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// UserInfo 用户信息
type UserInfo struct {
	UserId   interface{} `json:"UserId"` // 支持数字或字符串
	NickName string      `json:"NickName"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Platform string `json:"platform"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Data    struct {
		AccessToken string `json:"accessToken"`
		UserInfo    struct {
			UserId   interface{} `json:"userId"`
			NickName string      `json:"nickName"`
		} `json:"userInfo"`
	} `json:"data"`
}

// getUserInfo 获取用户信息
func (c *Client) getUserInfo(ctx context.Context) (*UserInfo, error) {
	var response struct {
		Code    int      `json:"Code"`
		Message string   `json:"Message"`
		Data    UserInfo `json:"Data"`
	}

	err := c.makeRequest(ctx, "GET", "/api/user/Account/getUserInfo", nil, &response)
	if err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取用户信息失败: %s", response.Message)
	}

	return &response.Data, nil
}

// SetToken 设置认证令牌
func (c *Client) SetToken(token string) {
	c.token = token
}

// IsTokenValid 检查token是否有效
func (c *Client) IsTokenValid(ctx context.Context) bool {
	_, err := c.getUserInfo(ctx)
	return err == nil
}

// GetInventory 获取库存
func (c *Client) GetInventory(ctx context.Context, refresh bool) ([]YouPinItem, error) {
	data := map[string]interface{}{
		"pageIndex": 1,
		"pageSize":  1000,
		"AppType":   4,
		"IsMerge":   0,
		"Sessionid": c.deviceToken,
	}

	if refresh {
		data["IsRefresh"] = true
		data["RefreshType"] = 2
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Message"`
		Data    struct {
			ItemsInfos []YouPinItem `json:"ItemsInfos"`
		} `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/commodity/Inventory/GetUserInventoryDataListV3", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取库存失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.ItemsInfos, nil
}

// GetSellList 获取在售物品列表 (用于内部服务逻辑)
func (c *Client) GetSellList(ctx context.Context) ([]YouPinOrder, error) {
	data := map[string]interface{}{
		"pageIndex":    1,
		"pageSize":     100,
		"whetherMerge": 0,
	}

	var shelf []YouPinOrder
	for {
		var response struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				CommodityInfoList []YouPinOrder `json:"commodityInfoList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/new/commodity/v1/commodity/list/sell", data, &response)
		if err != nil {
			return nil, fmt.Errorf("获取在售列表失败: %w", err)
		}

		if response.Code != 0 {
			break
		}

		for _, item := range response.Data.CommodityInfoList {
			// 只添加有ID的物品
			if item.ID != 0 {
				shelf = append(shelf, item)
			}
		}

		data["pageIndex"] = data["pageIndex"].(int) + 1
	}

	return shelf, nil
}

// GetSellListForAPI 获取在售物品列表 (用于API返回)
func (c *Client) GetSellListForAPI(ctx context.Context) ([]map[string]interface{}, error) {
	data := map[string]interface{}{
		"pageIndex":    1,
		"pageSize":     100,
		"whetherMerge": 0,
	}

	var shelf []map[string]interface{}
	for {
		var response struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				CommodityInfoList []map[string]interface{} `json:"commodityInfoList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/new/commodity/v1/commodity/list/sell", data, &response)
		if err != nil {
			return nil, fmt.Errorf("获取在售列表失败: %w", err)
		}

		if response.Code != 0 {
			break
		}

		for _, item := range response.Data.CommodityInfoList {
			// 只添加有steamAssetId的物品，与Steamauto保持一致
			if steamAssetId, exists := item["steamAssetId"]; exists && steamAssetId != nil {
				shelf = append(shelf, item)
			}
		}

		data["pageIndex"] = data["pageIndex"].(int) + 1
	}

	return shelf, nil
}

// GetWaitDeliverList 获取待发货列表
func (c *Client) GetWaitDeliverList(ctx context.Context) ([]YouPinOrder, error) {
	// 首先获取待办事项列表
	toDoListData := map[string]interface{}{
		"userId":    c.userId,
		"pageIndex": 1,
		"pageSize":  100,
		"Sessionid": c.deviceToken,
	}

	var toDoResponse struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    []struct {
			OrderNo       string `json:"orderNo"`
			CommodityName string `json:"commodityName"`
			Message       string `json:"message"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/todo/v1/orderTodo/list", toDoListData, &toDoResponse)
	if err != nil {
		return nil, fmt.Errorf("获取待办列表失败: %w", err)
	}

	var dataToReturn []YouPinOrder
	toDoList := make(map[string]struct {
		OrderNo       string
		CommodityName string
		Message       string
	})

	// 处理待办事项
	for _, order := range toDoResponse.Data {
		if strings.Contains(order.Message, "赠送") {
			// 跳过赠送订单
			continue
		} else if order.Message == "有买家下单，待您发送报价" {
			// 发送报价
			err := c.sendOffer(ctx, order.OrderNo)
			if err != nil {
				// 记录错误但继续处理
				continue
			}
			// 等待报价发送完成
			time.Sleep(2 * time.Second)
		} else {
			toDoList[order.OrderNo] = struct {
				OrderNo       string
				CommodityName string
				Message       string
			}{
				OrderNo:       order.OrderNo,
				CommodityName: order.CommodityName,
				Message:       order.Message,
			}
		}
	}

	// 获取销售列表中的报价ID
	if len(toDoList) > 0 {
		sellListData := map[string]interface{}{
			"keys":        "",
			"orderStatus": "140",
			"pageIndex":   1,
			"pageSize":    100,
		}

		var sellResponse struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Data    struct {
				OrderList []struct {
					OrderNo       string `json:"orderNo"`
					OfferType     string `json:"offerType"`
					TradeOfferID  string `json:"tradeOfferId"`
					ProductDetail struct {
						CommodityName string `json:"commodityName"`
					} `json:"productDetail"`
				} `json:"orderList"`
			} `json:"data"`
		}

		err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/sale/v1/sell/list", sellListData, &sellResponse)
		if err == nil && sellResponse.Code == 0 {
			for _, order := range sellResponse.Data.OrderList {
				if order.OfferType == "2" && order.TradeOfferID != "" {
					if _, exists := toDoList[order.OrderNo]; exists {
						delete(toDoList, order.OrderNo)
						dataToReturn = append(dataToReturn, YouPinOrder{
							OfferID:  order.TradeOfferID,
							ItemName: order.ProductDetail.CommodityName,
						})
					}
				}
			}
		}
	}

	// 如果还有未处理的订单，尝试其他方法获取报价ID
	if len(toDoList) > 0 {
		for orderNo := range toDoList {
			// 等待一下避免频繁请求
			time.Sleep(3 * time.Second)

			order, err := c.getOrderDetail(ctx, orderNo)
			if err == nil && order != nil {
				dataToReturn = append(dataToReturn, *order)
				delete(toDoList, orderNo)
			}
		}
	}

	return dataToReturn, nil
}

// sendOffer 发送报价
func (c *Client) sendOffer(ctx context.Context, orderNo string) error {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}

	err := c.makeRequest(ctx, "PUT", "/api/youpin/bff/trade/v1/order/sell/delivery/send-offer", data, &response)
	if err != nil {
		return err
	}

	if response.Code != 0 {
		return fmt.Errorf("发送报价失败: %s", response.Message)
	}

	return nil
}

// getOrderDetail 获取订单详情
func (c *Client) getOrderDetail(ctx context.Context, orderNo string) (*YouPinOrder, error) {
	data := map[string]interface{}{
		"orderId":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			OrderDetail struct {
				OfferID       string `json:"offerId"`
				TradeOfferID  string `json:"tradeOfferId"`
				ProductDetail struct {
					CommodityName string `json:"commodityName"`
				} `json:"productDetail"`
				Commodity struct {
					Name string `json:"name"`
				} `json:"commodity"`
			} `json:"orderDetail"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/order/v2/detail", data, &response)
	if err != nil {
		// 尝试另一个API
		return c.getOrderDetailV2(ctx, orderNo)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取订单详情失败: %s", response.Message)
	}

	if response.Data.OrderDetail.OfferID != "" {
		return &YouPinOrder{
			OfferID:  response.Data.OrderDetail.OfferID,
			ItemName: response.Data.OrderDetail.ProductDetail.CommodityName,
		}, nil
	}

	return nil, fmt.Errorf("订单详情中没有找到报价ID")
}

// getOrderDetailV2 获取订单详情（备用方法）
func (c *Client) getOrderDetailV2(ctx context.Context, orderNo string) (*YouPinOrder, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			TradeOfferID string `json:"tradeOfferId"`
			Commodity    struct {
				Name string `json:"name"`
			} `json:"commodity"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/query/detail", data, &response)
	if err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("获取订单详情失败: %s", response.Message)
	}

	if response.Data.TradeOfferID != "" {
		return &YouPinOrder{
			OfferID:  response.Data.TradeOfferID,
			ItemName: response.Data.Commodity.Name,
		}, nil
	}

	return nil, fmt.Errorf("订单详情中没有找到报价ID")
}

// GetMarketSalePrice 获取市场销售价格
func (c *Client) GetMarketSalePrice(ctx context.Context, templateID string) ([]YouPinMarketPrice, error) {
	data := map[string]interface{}{
		"pageIndex":  1,
		"pageSize":   10,
		"templateId": templateID,
	}

	var response struct {
		Code    int                 `json:"Code"`
		Message string              `json:"Message"`
		Data    []YouPinMarketPrice `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/homepage/pc/goods/market/queryOnSaleCommodityList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取市场价格失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data, nil
}

// SellItems 上架物品
func (c *Client) SellItems(ctx context.Context, items []YouPinSaleItem) error {
	data := map[string]interface{}{
		"GameId":    GameIDCSGO,
		"itemInfos": items,
		"Sessionid": c.deviceToken,
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Msg"`
		Data    []struct {
			AssetId string `json:"AssetId"`
			Status  int    `json:"Status"`
			Remark  string `json:"Remark"`
		} `json:"Data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/commodity/Inventory/SellInventoryWithLeaseV2", data, &response)
	if err != nil {
		return fmt.Errorf("上架物品失败: %w", err)
	}

	if response.Code != 0 && response.Message != "成功" {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	// 检查每个物品的上架状态
	for _, asset := range response.Data {
		if asset.Status != 1 {
			return fmt.Errorf("物品 %s 上架失败: %s", asset.AssetId, asset.Remark)
		}
	}

	return nil
}

// ChangeSalePrice 修改出售价格
func (c *Client) ChangeSalePrice(ctx context.Context, commodities []YouPinCommodity) error {
	data := map[string]interface{}{
		"GameId":     GameIDCSGO,
		"Commoditys": commodities,
		"Sessionid":  c.deviceToken,
	}

	var response YouPinPriceChangeResponse
	err := c.makeRequest(ctx, "PUT", "/api/commodity/Commodity/PriceChangeWithLeaseV2", data, &response)
	if err != nil {
		return fmt.Errorf("修改价格失败: %w", err)
	}

	if response.Code != 0 && response.Message != "成功" {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	// 检查修改结果
	for _, commodity := range response.Data.Commoditys {
		if commodity.IsSuccess != 1 {
			return fmt.Errorf("商品 %v 价格修改失败: %s", commodity.CommodityId, commodity.Message)
		}
	}

	return nil
}

// OffSale 下架商品
func (c *Client) OffSale(ctx context.Context, commodityIDs []string) error {
	data := map[string]interface{}{
		"Ids":                    strings.Join(commodityIDs, ","),
		"IsDeleteCommodityCache": 1,
		"IsForceOffline":         true,
		"Sessionid":              c.deviceToken,
	}

	var response YouPinSellResponse
	err := c.makeRequest(ctx, "PUT", "/api/commodity/Commodity/OffShelf", data, &response)
	if err != nil {
		return fmt.Errorf("下架商品失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	return nil
}

// GetUserNickname 获取用户昵称
func (c *Client) GetUserNickname() string {
	return c.nickname
}

// SendDeviceInfo 发送设备信息
func (c *Client) SendDeviceInfo(ctx context.Context) error {
	data := map[string]interface{}{
		"DeviceToken": c.deviceToken,
		"Sessionid":   c.deviceToken,
	}

	var response YouPinSellResponse
	err := c.makeRequest(ctx, "GET", "/api/common/ClientInfo/AndroidInfo", data, &response)
	if err != nil {
		return fmt.Errorf("发送设备信息失败: %w", err)
	}

	return nil
}

// SearchCommodities 统一使用安卓端悠悠有品搜索API
func (c *Client) SearchCommodities(ctx context.Context, keyword string, pageIndex int, pageSize int) ([]YouPinCommoditySearch, error) {
	data := map[string]interface{}{
		"keyword":   keyword,
		"pageIndex": pageIndex,
		"pageSize":  pageSize,
		"gameId":    GameIDCSGO,
	}

	var response struct {
		Code    int                     `json:"Code"`
		Message string                  `json:"Message"`
		Data    []YouPinCommoditySearch `json:"Data"`
	}

	// 安卓端 v3 搜索接口（与详情接口同前缀）
	// 兼容参数命名：同时传递 keyword 与 keys
	data["keys"] = keyword
	if err := c.makeRequest(ctx, "POST", "/api/homepage/v3/search/commodity", data, &response); err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}
	return response.Data, nil
}

// GetCommodityDetails 获取商品详情 - 直接返回商品列表
func (c *Client) GetCommodityDetails(ctx context.Context, templateId string) (*CommodityListResponse, error) {
	// 使用成功的商品列表API来获取商品详情
	templateIdInt := 0
	if id, err := strconv.Atoi(templateId); err == nil {
		templateIdInt = id
	} else {
		return nil, fmt.Errorf("无效的模板ID: %s", templateId)
	}

	// 直接返回商品列表，这就是用户想要的：看到市场上有哪些商家在卖这个商品
	response, err := c.GetCommodityList(ctx, templateIdInt, 1, 50, 1) // 获取50个在售商品
	if err != nil {
		return nil, fmt.Errorf("获取商品详情失败: %w", err)
	}

	return response, nil
}

// 磨损等级辅助函数
func getWearName(abrade float64) string {
	if abrade >= 0.45 {
		return "战痕累累"
	} else if abrade >= 0.38 {
		return "破损不堪"
	} else if abrade >= 0.15 {
		return "久经沙场"
	} else if abrade >= 0.07 {
		return "略有磨损"
	} else {
		return "崭新出厂"
	}
}

func getWearCode(abrade float64) string {
	if abrade >= 0.45 {
		return "BS"
	} else if abrade >= 0.38 {
		return "WW"
	} else if abrade >= 0.15 {
		return "FT"
	} else if abrade >= 0.07 {
		return "MW"
	} else {
		return "FN"
	}
}

func getMinAbradeForWear(abrade float64) float64 {
	if abrade >= 0.45 {
		return 0.45
	} else if abrade >= 0.38 {
		return 0.38
	} else if abrade >= 0.15 {
		return 0.15
	} else if abrade >= 0.07 {
		return 0.07
	} else {
		return 0.0
	}
}

func getMaxAbradeForWear(abrade float64) float64 {
	if abrade >= 0.45 {
		return 1.0
	} else if abrade >= 0.38 {
		return 0.45
	} else if abrade >= 0.15 {
		return 0.38
	} else if abrade >= 0.07 {
		return 0.15
	} else {
		return 0.07
	}
}

// GetMarketSaleList 获取市场销售列表（不同磨损的物品）
func (c *Client) GetMarketSaleList(ctx context.Context, templateId string, pageIndex int, pageSize int, minAbrade float64, maxAbrade float64) ([]YouPinMarketItem, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"pageIndex":  pageIndex,
		"pageSize":   pageSize,
	}

	// 按照Steamauto的格式，磨损值作为字符串传递，并且只在指定时添加
	if minAbrade > 0 {
		data["minAbrade"] = fmt.Sprintf("%.6f", minAbrade)
	}
	if maxAbrade < 1 && maxAbrade > 0 {
		data["maxAbrade"] = fmt.Sprintf("%.6f", maxAbrade)
	}

	var response struct {
		Code    int    `json:"Code"`
		Message string `json:"Message"`
		Data    struct {
			CommodityList []YouPinMarketItem `json:"CommodityList"`
			TotalCount    int                `json:"TotalCount"`
		} `json:"Data"`
	}

	// 使用Steamauto验证过的PC端API
	err := c.makeRequest(ctx, "POST", "/api/homepage/pc/goods/market/queryOnSaleCommodityList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取市场销售列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.CommodityList, nil
}

// GetPurchaseOrderList 获取求购订单列表
func (c *Client) GetPurchaseOrderList(ctx context.Context, templateId string, pageIndex int, pageSize int, minAbrade float64, maxAbrade float64) ([]YouPinPurchaseOrder, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"pageIndex":  pageIndex,
		"pageSize":   pageSize,
		"minAbrade":  minAbrade,
		"maxAbrade":  maxAbrade,
		"typeId":     -1, // -1表示所有类型
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Data    struct {
			PurchaseOrderList []YouPinPurchaseOrder `json:"purchaseOrderList"`
			TotalCount        int                   `json:"totalCount"`
		} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseOrderList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购订单列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Message)
	}

	return response.Data.PurchaseOrderList, nil
}

// CreatePurchaseOrder 创建求购订单
func (c *Client) CreatePurchaseOrder(ctx context.Context, templateId string, templateHashName string, commodityName string, purchasePrice float64, purchaseNum int, minAbrade float64, maxAbrade float64) error {
	data := map[string]interface{}{
		"templateId":       templateId,
		"templateHashName": templateHashName,
		"commodityName":    commodityName,
		"purchasePrice":    purchasePrice,
		"purchaseNum":      purchaseNum,
		"minAbrade":        minAbrade,
		"maxAbrade":        maxAbrade,
		"orderNo":          "",
		"supplyQuantity":   0,
		"Sessionid":        c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/publishPurchaseOrder", data, &response)
	if err != nil {
		return fmt.Errorf("创建求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("API返回错误: %s", response.Message)
	}

	return nil
}

// BuyFromMarket 从市场直接购买
func (c *Client) BuyFromMarket(ctx context.Context, commodityId string, price float64) error {
	data := map[string]interface{}{
		"commodityId": commodityId,
		"price":       price,
		"Sessionid":   c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/buy", data, &response)
	if err != nil {
		return fmt.Errorf("市场购买失败: %w", err)
	}

	fmt.Printf("DEBUG: 市场购买响应 - Code: %d, Msg: %s\n", response.Code, response.Message)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Message == "成功" {
		fmt.Printf("DEBUG: 市场购买成功 (Code: %d)\n", response.Code)
		return nil
	}

	// 只有当msg不是"成功"时才返回错误
	return fmt.Errorf("市场购买失败: %s", response.Message)
}

// BuyFromMarketWithBalance 使用余额从市场直接购买
func (c *Client) BuyFromMarketWithBalance(ctx context.Context, commodityId string, price float64) error {
	data := map[string]interface{}{
		"commodityId":   commodityId,
		"price":         price,
		"paymentMethod": "balance", // 指定使用余额支付
		"payType":       1,         // 1表示余额支付
		"Sessionid":     c.deviceToken,
	}

	var response struct {
		Code    int         `json:"code"`
		Message string      `json:"msg"`
		Data    interface{} `json:"data"`
	}

	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/buy", data, &response)
	if err != nil {
		return fmt.Errorf("余额购买失败: %w", err)
	}

	fmt.Printf("DEBUG: 余额购买响应 - Code: %d, Msg: %s\n", response.Code, response.Message)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Message == "成功" {
		fmt.Printf("DEBUG: 余额购买成功 (Code: %d)\n", response.Code)
		return nil
	}

	// 只有当msg不是"成功"时才返回错误
	return fmt.Errorf("余额购买失败: %s", response.Message)
}

// 基于HAR文件分析实现的多步骤YouPin购买流程

// MultiStepBuyWithBalance 多步骤余额购买流程（基于HAR分析）
func (c *Client) MultiStepBuyWithBalance(ctx context.Context, commodityId int64, price float64) (*YouPinMultiStepBuyResponse, error) {
	priceStr := fmt.Sprintf("%.2f", price)

	// 第1步：订单预检查 - 暂时跳过，因为API返回码7000002但消息是"成功"的问题
	fmt.Printf("DEBUG: 跳过订单预检查步骤，直接进行后续购买流程\n")
	// preCheckResp, err := c.orderPreCheck(ctx, commodityId)
	// if err != nil {
	// 	return &YouPinMultiStepBuyResponse{
	// 		Success: false,
	// 		Message: "订单预检查失败",
	// 		Error:   err.Error(),
	// 		Step:    "precheck",
	// 	}, err
	// }

	// if !preCheckResp.Data.CanBuy {
	// 	return &YouPinMultiStepBuyResponse{
	// 		Success: false,
	// 		Message: preCheckResp.Data.Message,
	// 		Step:    "precheck",
	// 	}, fmt.Errorf("商品不可购买: %s", preCheckResp.Data.Message)
	// }

	// 第2步：创建订单
	createOrderResp, err := c.createOrder(ctx, commodityId, priceStr)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "创建订单失败",
			Error:   err.Error(),
			Step:    "create_order",
		}, err
	}

	orderNo := createOrderResp.Data.OrderNo
	if orderNo == "" {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "订单号为空",
			Step:    "create_order",
		}, fmt.Errorf("创建订单失败: 订单号为空")
	}

	// 第3步：支付确认
	_, err = c.confirmPayment(ctx, orderNo)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "支付确认失败",
			Error:   err.Error(),
			OrderNo: orderNo,
			Step:    "payment",
		}, err
	}

	// 第4步：获取订单状态
	statusResp, err := c.getOrderStatus(ctx, orderNo)
	if err != nil {
		return &YouPinMultiStepBuyResponse{
			Success: false,
			Message: "获取订单状态失败",
			Error:   err.Error(),
			OrderNo: orderNo,
			Step:    "status_check",
		}, err
	}

	// 第5步：获取报价状态（如HAR所示的最后一步）
	_, err = c.getOfferStatus(ctx, orderNo)
	if err != nil {
		// 报价状态获取失败不影响整体购买流程，记录日志即可
		fmt.Printf("获取报价状态失败（非关键错误）: %v\n", err)
	}

	return &YouPinMultiStepBuyResponse{
		Success: true,
		Message: "购买流程完成",
		OrderNo: orderNo,
		Status:  statusResp.Data.Status,
		Step:    "completed",
	}, nil
}

// orderPreCheck 订单预检查
func (c *Client) orderPreCheck(ctx context.Context, commodityId int64) (*YouPinOrderPreCheckResponse, error) {
	data := map[string]interface{}{
		"commodityId": commodityId,
		"Sessionid":   c.deviceToken,
	}

	var response YouPinOrderPreCheckResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/pre-check", data, &response)
	if err != nil {
		return nil, fmt.Errorf("订单预检查请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 订单预检查响应 - Code: %d, Msg: %s, CanBuy: %v\n", response.Code, response.Msg, response.Data.CanBuy)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 订单预检查成功 (Code: %d, CanBuy: %v)\n", response.Code, response.Data.CanBuy)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("订单预检查失败: %s", response.Msg)
}

// createOrder 创建订单
func (c *Client) createOrder(ctx context.Context, commodityId int64, price string) (*YouPinCreateOrderResponse, error) {
	data := map[string]interface{}{
		"commodityId":   strconv.FormatInt(commodityId, 10),
		"buyerUserId":   c.userId,
		"price":         price,
		"paymentMethod": "balance", // 余额支付
		"businessType":  "1",       // 基于HAR分析的业务类型
		"gameId":        "730",     // CS:GO游戏ID
		"Sessionid":     c.deviceToken,
	}

	var response YouPinCreateOrderResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/create", data, &response)
	if err != nil {
		return nil, fmt.Errorf("创建订单请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 创建订单响应 - Code: %d, Msg: %s, OrderNo: %s\n", response.Code, response.Msg, response.Data.OrderNo)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		if response.Data.OrderNo != "" {
			fmt.Printf("DEBUG: 创建订单成功 (Code: %d, OrderNo: %s)\n", response.Code, response.Data.OrderNo)
			return &response, nil
		} else {
			// msg显示成功但订单号为空，这种情况也当作成功处理，但记录警告
			fmt.Printf("WARNING: 创建订单API返回成功但订单号为空 (Code: %d)\n", response.Code)
			return &response, nil
		}
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("创建订单失败: %s", response.Msg)
}

// confirmPayment 确认支付
func (c *Client) confirmPayment(ctx context.Context, orderNo string) (*YouPinPaymentConfirmResponse, error) {
	data := map[string]interface{}{
		"orderNo":       orderNo,
		"paymentMethod": "balance", // 余额支付
		"userId":        c.userId,
		"Sessionid":     c.deviceToken,
	}

	var response YouPinPaymentConfirmResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/pay/order/confirm", data, &response)
	if err != nil {
		return nil, fmt.Errorf("支付确认请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 支付确认响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 支付确认成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("支付确认失败: %s", response.Msg)
}

// getOrderStatus 获取订单状态
func (c *Client) getOrderStatus(ctx context.Context, orderNo string) (*YouPinOrderStatusResponse, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"userId":    c.userId,
		"Sessionid": c.deviceToken,
	}

	var response YouPinOrderStatusResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/payment/v1/pay/order/status", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取订单状态请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 获取订单状态响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 获取订单状态成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("获取订单状态失败: %s", response.Msg)
}

// getOfferStatus 获取报价状态（基于HAR分析的最后一步）
func (c *Client) getOfferStatus(ctx context.Context, orderNo string) (*GetOfferStatusResponse, error) {
	data := map[string]interface{}{
		"orderNo":   orderNo,
		"userId":    c.userId,
		"Sessionid": c.deviceToken,
	}

	var response GetOfferStatusResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/v1/order/sell/delivery/get-offer-status", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取报价状态请求失败: %w", err)
	}

	fmt.Printf("DEBUG: 获取报价状态响应 - Code: %d, Msg: %s\n", response.Code, response.Msg)

	// 根据msg是否为"成功"来判定请求是否成功，不依赖code
	if response.Msg == "成功" {
		fmt.Printf("DEBUG: 获取报价状态成功 (Code: %d)\n", response.Code)
		return &response, nil
	}

	// 只有当msg不是"成功"时才返回错误
	return nil, fmt.Errorf("获取报价状态失败: %s", response.Msg)
}

// makeRequest 发起HTTP请求
func (c *Client) makeRequest(ctx context.Context, method, path string, data interface{}, result interface{}) error {
	// 如果使用开放平台API，使用签名请求
	if c.useOpenAPI {
		return c.makeOpenAPIRequest(ctx, method, path, data, result)
	}
	// 否则使用传统Token认证
	return c.makeRequestWithGzip(ctx, method, path, data, result, false)
}

// makeRequestWithGzip 发起HTTP请求，可选择是否使用gzip压缩
func (c *Client) makeRequestWithGzip(ctx context.Context, method, path string, data interface{}, result interface{}, useGzip bool) error {
	var body io.Reader
	url := c.baseURL + path

	// 根据方法类型处理参数
	if method == "GET" && data != nil {
		// GET请求使用URL参数
		params := make([]string, 0)
		if dataMap, ok := data.(map[string]interface{}); ok {
			for k, v := range dataMap {
				params = append(params, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(params) > 0 {
			url += "?" + strings.Join(params, "&")
		}
	} else if data != nil {
		// POST/PUT请求使用JSON body
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("序列化请求数据失败: %w", err)
		}

		if useGzip {
			// 对JSON数据进行gzip压缩
			var buf bytes.Buffer
			gzWriter := gzip.NewWriter(&buf)
			if _, err := gzWriter.Write(jsonData); err != nil {
				return fmt.Errorf("gzip压缩失败: %w", err)
			}
			if err := gzWriter.Close(); err != nil {
				return fmt.Errorf("关闭gzip写入器失败: %w", err)
			}
			body = &buf
		} else {
			body = bytes.NewBuffer(jsonData)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 根据抓包信息更新headers格式（模拟Android客户端）
	req.Header["Content-Type"] = []string{"application/json"}
	req.Header["User-Agent"] = []string{"okhttp/3.14.9"}
	req.Header["DeviceToken"] = []string{c.deviceToken}
	req.Header["DeviceId"] = []string{c.deviceID}
	req.Header["platform"] = []string{"android"}
	req.Header["App-Version"] = []string{"5.37.1"}
	req.Header["uk"] = []string{"5FQFWiQh8VvtSm0krHaYs52HWGSqA0v4UVcWASmLbSD68mdWzxo3oSoRtbSgwY91L"}
	req.Header["deviceUk"] = []string{"5FQIZE57VAGa7uQBapxU70o3PHzUYIUevEmrT53gRd8hMLiEMafT7TmLexlKfk51I"}
	req.Header["AppType"] = []string{"4"}
	req.Header["Authorization"] = []string{"Bearer " + c.token}
	// 发起请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发起请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应，支持gzip解压
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP错误: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	if result != nil {
		err = json.Unmarshal(respBody, result)
		if err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	// 检查登录状态
	if strings.Contains(string(respBody), "84101") {
		return fmt.Errorf("登录状态失效，请重新登录")
	}

	return nil
}

// signRequest 对请求参数进行签名
func (c *Client) signRequest(params map[string]interface{}, secret string) string {
	// 将参数转换为字符串并排序
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建签名字符串
	var signParts []string
	for _, k := range keys {
		v := params[k]
		var valueStr string
		switch val := v.(type) {
		case string:
			valueStr = val
		case int:
			valueStr = strconv.Itoa(val)
		case int64:
			valueStr = strconv.FormatInt(val, 10)
		case float64:
			valueStr = strconv.FormatFloat(val, 'f', -1, 64)
		case bool:
			valueStr = strconv.FormatBool(val)
		default:
			valueStr = fmt.Sprintf("%v", val)
		}
		signParts = append(signParts, fmt.Sprintf("%s=%s", k, valueStr))
	}

	signStr := strings.Join(signParts, "&") + "&key=" + secret

	// 计算MD5
	hash := md5.Sum([]byte(signStr))
	return fmt.Sprintf("%x", hash)
}

// SendSMSCode 发送短信验证码
func SendSMSCode(ctx context.Context, phone string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	// 生成会话ID
	sessionId := generateRandomString(32)

	data := map[string]interface{}{
		"Area":      86,        // 中国区号
		"Mobile":    phone,     // 手机号
		"Sessionid": sessionId, // 会话ID
		"Code":      "",        // 空验证码
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化请求数据失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/api/user/Auth/SendSignInSmsCode", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头，模拟Android APP
	deviceInfo := map[string]interface{}{
		"deviceId":      sessionId,
		"deviceType":    sessionId,
		"hasSteamApp":   1,
		"requestTag":    strings.ToUpper(generateRandomString(32)),
		"systemName ":   "Android",
		"systemVersion": "15",
	}
	deviceInfoJson, _ := json.Marshal(deviceInfo)

	req.Header["uk"] = []string{generateRandomString(65)}
	req.Header["authorization"] = []string{"Bearer "}
	req.Header["Content-Type"] = []string{"application/json; charset=utf-8"}
	req.Header["User-Agent"] = []string{"okhttp/3.14.9"}
	req.Header["App-Version"] = []string{"5.28.3"}
	req.Header["AppType"] = []string{"4"}
	req.Header["deviceType"] = []string{"1"}
	req.Header["package-type"] = []string{"uuyp"}
	req.Header["DeviceToken"] = []string{sessionId}
	req.Header["DeviceId"] = []string{sessionId}
	req.Header["platform"] = []string{"android"}
	req.Header["accept-encoding"] = []string{"gzip"}
	req.Header["Gameid"] = []string{"730"}
	req.Header["Device-Info"] = []string{string(deviceInfoJson)}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理gzip压缩
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var response struct {
		Code int    `json:"Code"`
		Msg  string `json:"Msg"`
	}

	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Code != 200 {
		return fmt.Errorf("发送验证码失败: %s", response.Msg)
	}

	return nil
}

// LoginWithPhone 使用手机号和验证码登录
func LoginWithPhone(ctx context.Context, phone, code string) (string, *UserInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// 生成会话ID
	sessionId := generateRandomString(32)

	data := map[string]interface{}{
		"Area":       86,        // 中国区号
		"Code":       code,      // 验证码
		"DeviceName": sessionId, // 设备名称
		"Sessionid":  sessionId, // 会话ID
		"Mobile":     phone,     // 手机号
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", nil, fmt.Errorf("序列化请求数据失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/api/user/Auth/SmsSignIn", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头，模拟Android APP
	deviceInfo := map[string]interface{}{
		"deviceId":      sessionId,
		"deviceType":    sessionId,
		"hasSteamApp":   1,
		"requestTag":    strings.ToUpper(generateRandomString(32)),
		"systemName ":   "Android",
		"systemVersion": "15",
	}
	deviceInfoJson, _ := json.Marshal(deviceInfo)

	req.Header["uk"] = []string{generateRandomString(65)}
	req.Header["authorization"] = []string{"Bearer "}
	req.Header["Content-Type"] = []string{"application/json; charset=utf-8"}
	req.Header["User-Agent"] = []string{"okhttp/3.14.9"}
	req.Header["App-Version"] = []string{"5.28.3"}
	req.Header["AppType"] = []string{"4"}
	req.Header["deviceType"] = []string{"1"}
	req.Header["package-type"] = []string{"uuyp"}
	req.Header["DeviceToken"] = []string{sessionId}
	req.Header["DeviceId"] = []string{sessionId}
	req.Header["platform"] = []string{"android"}
	req.Header["accept-encoding"] = []string{"gzip"}
	req.Header["Gameid"] = []string{"730"}
	req.Header["Device-Info"] = []string{string(deviceInfoJson)}

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理gzip压缩
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", nil, fmt.Errorf("创建gzip读取器失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var response LoginResponse
	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return "", nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Code != 0 {
		return "", nil, fmt.Errorf("登录失败: %s", response.Message)
	}

	userInfo := &UserInfo{
		UserId:   response.Data.UserInfo.UserId,
		NickName: response.Data.UserInfo.NickName,
	}

	return response.Data.AccessToken, userInfo, nil
}

// SearchItems 搜索商品 - 完全按抓包信息复刻
func (c *Client) SearchItems(ctx context.Context, keywords string, pageIndex int, pageSize int, sortType int) (*SearchNewListResponse, error) {
	// 设置默认值 - 支持动态分页
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20 // 默认20条
	}
	// 限制最大分页大小，避免服务器压力
	if pageSize > 100 {
		pageSize = 100
	}

	// 使用正确的搜索API接口
	data := map[string]interface{}{
		"filterMap":          map[string]interface{}{},
		"gameId":             730,
		"keyWords":           keywords,
		"listSortType":       0,
		"listType":           10,
		"pageCode":           "MARKET_PAGE_BUY_COMMODITY_PAGE",
		"pageIndex":          pageIndex,
		"pageSize":           pageSize,
		"pageSourceCode":     "PG3000002",
		"propertyFilterTags": []interface{}{},
		"sortType":           sortType,
		"stickerAbrade":      0,
		"stickersIsSort":     false,
		"Sessionid":          c.deviceToken,
	}

	var response SearchNewListResponse
	err := c.makeRequest(ctx, "POST", "/api/homepage/search/new/list", data, &response)
	if err != nil {
		return nil, fmt.Errorf("搜索商品失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// GetCommodityList 获取出售商品列表
func (c *Client) GetCommodityList(ctx context.Context, templateId int, pageIndex int, pageSize int, sortType int) (*CommodityListResponse, error) {
	// 根据抓包信息补全所有必需参数
	data := map[string]interface{}{
		"commodityId":             fmt.Sprintf("%d", templateId), // 添加缺失的 commodityId 参数
		"templateId":              fmt.Sprintf("%d", templateId),
		"abrasion":                []interface{}{},
		"phase":                   []interface{}{},
		"itemCategory":            "weapon",
		"rarity":                  []interface{}{},
		"quality":                 []interface{}{},
		"price":                   "",
		"isStattrak":              "",
		"category":                "730",
		"exterior":                []interface{}{},
		"minPrice":                "",
		"maxPrice":                "",
		"sortType":                "",
		"sortBy":                  "",
		"sortByPrice":             "",
		"currentPage":             fmt.Sprintf("%d", pageIndex),
		"currentPageSize":         fmt.Sprintf("%d", pageSize),
		"minAutoDelivery":         "",
		"forceSeller":             "",
		"language":                "cn",
		"fromAccountId":           "",
		"autoDelivery":            0,
		"hasSold":                 "true",
		"haveBuZhangType":         0,
		"isDialogMarket":          false,
		"isMultipleZone":          0,
		"listSortType":            "1",
		"listType":                10,
		"mergeFlag":               0,
		"pageIndex":               pageIndex,
		"pageSize":                pageSize,
		"pageSourceCode":          "PG3000003",
		"presaleMoreZones":        2,
		"sortTypeKey":             "",
		"sourceChannel":           "",
		"status":                  "20",
		"stickerAbrade":           0,
		"stickersIsSort":          false,
		"ultraLongLeaseMoreZones": 0,
		"userId":                  c.userId,
		"Sessionid":               c.deviceToken,
	}

	var response CommodityListResponse
	err := c.makeRequest(ctx, "POST", "/api/homepage/v3/detail/commodity/list/sell", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取商品列表失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	// 修正 TotalCount - 使用根级别的值
	if response.Data.TotalCount == 0 && response.TotalCount > 0 {
		response.Data.TotalCount = response.TotalCount
	}

	// 使用commodityNo作为commodityId的值，保持字段名为commodityId
	for i := range response.Data.CommodityList {
		commodity := &response.Data.CommodityList[i]
		// 优先使用 commodityNo 作为 commodityId 的值
		if commodity.CommodityNo != "" {
			commodity.CommodityId = commodity.CommodityNo
		} else if commodity.CommodityId == "" {
			// 如果 commodityNo 也为空，则生成临时ID
			if commodity.TemplateId > 0 {
				commodity.CommodityId = fmt.Sprintf("%d_%s_%s", commodity.TemplateId,
					strings.ReplaceAll(commodity.Price, ".", ""),
					strings.ReplaceAll(commodity.Abrade, ".", ""))
			}
		}
	}

	fmt.Printf("DEBUG: GetCommodityList response - Code: %d, Msg: %s, TotalCount: %d, CommodityList length: %d\n",
		response.Code, response.Msg, response.Data.TotalCount, len(response.Data.CommodityList))

	// 验证修复结果
	for i, commodity := range response.Data.CommodityList {
		if commodity.CommodityId == "" {
			fmt.Printf("WARNING: CommodityId仍为空 - Index: %d, TemplateId: %d, CommodityNo: '%s', Price: %s\n",
				i, commodity.TemplateId, commodity.CommodityNo, commodity.Price)
		}
	}

	return &response, nil
}

// GetTemplateFilterConfig 获取模板筛选配置
func (c *Client) GetTemplateFilterConfig(ctx context.Context, templateId int) (*FilterConfigResponse, error) {
	data := map[string]interface{}{
		"templateId": templateId,
		"Sessionid":  c.deviceToken,
	}

	var response FilterConfigResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplateFilterConfigV2", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取筛选配置失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// 求购相关API方法 - 基于HAR抓包分析

// GetTemplatePurchaseInfo 获取物品求购信息
func (c *Client) GetTemplatePurchaseInfo(ctx context.Context, templateId string) (*GetTemplatePurchaseInfoResponse, error) {
	data := map[string]interface{}{
		"templateId": templateId,
	}

	var response GetTemplatePurchaseInfoResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseInfo", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购信息失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// PrePurchaseOrderCheck 预检查求购订单
func (c *Client) PrePurchaseOrderCheck(ctx context.Context, req PrePurchaseOrderCheckRequest) (*PrePurchaseOrderCheckResponse, error) {
	var response PrePurchaseOrderCheckResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/prePurchaseOrderCheck", req, &response)
	if err != nil {
		return nil, fmt.Errorf("预检查求购订单失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// SavePurchaseOrder 创建求购订单
func (c *Client) SavePurchaseOrder(ctx context.Context, req SavePurchaseOrderRequest) (*SavePurchaseOrderResponse, error) {
	var response SavePurchaseOrderResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/savePurchaseOrder", req, &response)
	if err != nil {
		return nil, fmt.Errorf("创建求购订单失败: %w", err)
	}

	// 根据HAR分析，处理各种情况
	// code == 200210008: 重复求购确认（是否取消并重新发起）
	if response.Code == 200210008 {
		return &response, fmt.Errorf("REPEAT_ORDER_CONFIRM: %s", response.Msg)
	}

	// code == 200210014: 价格高于在售价格的警告
	if response.Code == 200210014 {
		return &response, fmt.Errorf("PRICE_WARNING: %s", response.Msg)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// GetPurchaseOrderDetail 获取求购订单详情
func (c *Client) GetPurchaseOrderDetail(ctx context.Context, orderNo string) (*GetPurchaseOrderDetailResponse, error) {
	data := map[string]interface{}{
		"orderNo": orderNo,
	}

	var response GetPurchaseOrderDetailResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getPurchaseOrderDetail", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购订单详情失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// GetPurchaseSupplyOrderList 获取求购供应订单列表
func (c *Client) GetPurchaseSupplyOrderList(ctx context.Context, purchaseNo string) (*GetPurchaseSupplyOrderListResponse, error) {
	data := map[string]interface{}{
		"purchaseNo": purchaseNo,
	}

	var response GetPurchaseSupplyOrderListResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getPurchaseSupplyOrderList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购供应订单列表失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// GetTemplatePurchaseOrderList 获取物品求购列表
func (c *Client) GetTemplatePurchaseOrderList(ctx context.Context, templateId int, pageIndex int, pageSize int) (*GetTemplatePurchaseOrderListResponse, error) {
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	data := map[string]interface{}{
		"pageIndex":        pageIndex,
		"pageSize":         pageSize,
		"showMaxPriceFlag": false,
		"templateId":       templateId,
		"Sessionid":        c.deviceToken,
	}

	var response GetTemplatePurchaseOrderListResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseOrderList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购列表失败: %w", err)
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// CreatePurchaseOrderComplete 完整的求购流程（简化版API）
func (c *Client) CreatePurchaseOrderComplete(ctx context.Context, templateId string, templateHashName string, commodityName string, purchasePrice float64, purchaseNum int, referencePrice string, minSellPrice string, maxPurchasePrice string, autoReceived bool) (*SavePurchaseOrderResponse, error) {
	// 1. 获取求购信息
	_, err := c.GetTemplatePurchaseInfo(ctx, templateId)
	if err != nil {
		return nil, fmt.Errorf("获取求购信息失败: %w", err)
	}

	// 2. 预检查订单
	totalAmount := purchasePrice * float64(purchaseNum)
	minSell, _ := strconv.ParseFloat(minSellPrice, 64)
	maxPurchase, _ := strconv.ParseFloat(maxPurchasePrice, 64)

	preCheckReq := PrePurchaseOrderCheckRequest{
		SpecialStyleObj:  make(map[string]interface{}),
		IsCheckMaxPrice:  false,
		TemplateHashName: templateHashName,
		TotalAmount:      totalAmount,
		ReferencePrice:   referencePrice,
		PurchasePrice:    purchasePrice,
		PurchaseNum:      purchaseNum,
		DiscountAmount:   0,
		MinSellPrice:     minSell,
		MaxPurchasePrice: maxPurchase,
		TemplateId:       templateId,
	}

	// 如果开启自动收货，在预检查时也要添加服务代码
	if autoReceived {
		preCheckReq.IncrementServiceCode = []int{1001}
	}

	_, err = c.PrePurchaseOrderCheck(ctx, preCheckReq)
	if err != nil {
		return nil, fmt.Errorf("预检查失败: %w", err)
	}

	// 3. 创建求购订单（首次尝试，不确认支付）
	templateIdInt, _ := strconv.Atoi(templateId)
	saveReq := SavePurchaseOrderRequest{
		TemplateId:            templateIdInt,
		TemplateHashName:      templateHashName,
		CommodityName:         commodityName,
		ReferencePrice:        referencePrice,
		MinSellPrice:          minSellPrice,
		MaxPurchasePrice:      maxPurchasePrice,
		PurchasePrice:         purchasePrice,
		PurchaseNum:           purchaseNum,
		NeedPaymentAmount:     totalAmount,
		TotalAmount:           totalAmount,
		TemplateName:          commodityName,
		PriceDifference:       0,
		DiscountAmount:        0,
		PayConfirmFlag:        false,
		RepeatOrderCancelFlag: false,
	}

	// 如果开启自动收货，添加服务代码
	if autoReceived {
		saveReq.IncrementServiceCode = []int{1001}
	}

	response, err := c.SavePurchaseOrder(ctx, saveReq)
	if err != nil {
		// 如果遇到重复订单确认，自动同意取消重新发起
		if strings.Contains(err.Error(), "REPEAT_ORDER_CONFIRM") {
			saveReq.RepeatOrderCancelFlag = true
			response, err = c.SavePurchaseOrder(ctx, saveReq)
			if err != nil {
				// 如果还是出错，可能是价格警告
				if strings.Contains(err.Error(), "PRICE_WARNING") {
					saveReq.PayConfirmFlag = true
					response, err = c.SavePurchaseOrder(ctx, saveReq)
					if err != nil {
						return nil, fmt.Errorf("三次确认创建求购订单失败: %w", err)
					}
				} else {
					return nil, fmt.Errorf("重复订单确认后失败: %w", err)
				}
			}
		} else if strings.Contains(err.Error(), "PRICE_WARNING") {
			// 如果遇到价格警告，则需要二次确认
			saveReq.PayConfirmFlag = true
			response, err = c.SavePurchaseOrder(ctx, saveReq)
			if err != nil {
				return nil, fmt.Errorf("价格警告确认后失败: %w", err)
			}
		} else {
			return nil, fmt.Errorf("创建求购订单失败: %w", err)
		}
	}

	return response, nil
}

// SearchPurchaseOrderList 获取当前账号的求购列表
func (c *Client) SearchPurchaseOrderList(ctx context.Context, pageIndex, pageSize, status int) (*SearchPurchaseOrderListResponse, error) {
	data := SearchPurchaseOrderListRequest{
		PageIndex: pageIndex,
		PageSize:  pageSize,
		Status:    status,
		Sessionid: c.deviceToken,
	}

	var response SearchPurchaseOrderListResponse
	// 使用gzip压缩，根据抓包数据，这个API需要gzip
	err := c.makeRequestWithGzip(ctx, "POST", "/api/youpin/bff/trade/purchase/order/searchPurchaseOrderList", data, &response, true)
	if err != nil {
		fmt.Printf("[SearchPurchaseOrderList] Request failed: %v\n", err)
		return nil, err
	}
	fmt.Printf("[SearchPurchaseOrderList] Response: code=%d, msg=%s, data_count=%d\n", response.Code, response.Msg, len(response.Data))
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// DeletePurchaseOrder 删除求购订单
func (c *Client) DeletePurchaseOrder(ctx context.Context, orderNoList []string) (*DeletePurchaseOrderResponse, error) {
	data := DeletePurchaseOrderRequest{
		OrderNoList: orderNoList,
		Sessionid:   c.deviceToken,
	}

	var response DeletePurchaseOrderResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/deletePurchaseOrder", data, &response)
	if err != nil {
		return nil, err
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// UpdatePurchaseOrder 修改求购订单
func (c *Client) UpdatePurchaseOrder(ctx context.Context, req UpdatePurchaseOrderRequest) (*UpdatePurchaseOrderResponse, error) {
	var response UpdatePurchaseOrderResponse
	err := c.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/updatePurchaseOrder", req, &response)
	if err != nil {
		return nil, err
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}
	return &response, nil
}

// makeOpenAPIRequest 发起开放平台签名请求
func (c *Client) makeOpenAPIRequest(ctx context.Context, method, path string, data interface{}, result interface{}) error {
	// 1. 准备请求参数
	var params map[string]interface{}
	if data != nil {
		// 将data转换为map
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("序列化请求数据失败: %w", err)
		}
		if err := json.Unmarshal(dataBytes, &params); err != nil {
			return fmt.Errorf("转换请求数据失败: %w", err)
		}
	} else {
		params = make(map[string]interface{})
	}

	// 2. 添加timestamp（GMT+8北京时间）
	location, _ := time.LoadLocation("Asia/Shanghai")
	timestamp := time.Now().In(location).Format("2006-01-02 15:04:05")

	// 3. 添加签名
	if err := c.rsaSigner.AddSignatureToParams(params, timestamp); err != nil {
		return fmt.Errorf("签名失败: %w", err)
	}

	// 4. 序列化请求体
	jsonData, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("序列化请求数据失败: %w", err)
	}

	// 5. 构建请求URL（开放平台使用不同的baseURL）
	url := c.openAPIURL + path

	// 6. 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 7. 设置请求头（开放平台要求）
	req.Header["Content-Type"] = []string{"application/json"}
	req.Header["Accept"] = []string{"application/json"}

	// 8. 发起请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发起请求失败: %w", err)
	}
	defer resp.Body.Close()
	// 9. 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 10. 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP错误: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 11. 解析响应
	if result != nil {
		err = json.Unmarshal(respBody, result)
		if err != nil {
			return fmt.Errorf("解析响应失败: %w, 响应内容: %s", err, string(respBody))
		}
	}

	return nil
}

// BatchGetOnSaleCommodityInfo 批量查询在售商品价格（开放平台API）
// requestList: 批量请求参数列表，每项包含templateId或templateHashName
func (c *Client) BatchGetOnSaleCommodityInfo(ctx context.Context, requestList []map[string]interface{}) (*BatchGetOnSaleCommodityInfoResponse, error) {
	// 验证是否使用开放平台API
	if !c.useOpenAPI {
		return nil, fmt.Errorf("此接口仅支持开放平台API认证方式")
	}

	// 验证请求列表
	if len(requestList) == 0 {
		return nil, fmt.Errorf("请求列表不能为空")
	}
	if len(requestList) > 200 {
		return nil, fmt.Errorf("请求列表数量不能超过200")
	}

	// 构建请求数据
	data := map[string]interface{}{
		"requestList": requestList,
	}

	var response BatchGetOnSaleCommodityInfoResponse
	err := c.makeRequest(ctx, "POST", "/open/v1/api/batchGetOnSaleCommodityInfo", data, &response)
	if err != nil {
		return nil, fmt.Errorf("批量查询在售商品价格失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}
