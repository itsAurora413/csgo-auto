package youpin

import (
	"context"
	"fmt"
	"time"
)

// ===========================
// 硬编码的 OpenAPI 凭证
// ===========================

// YoupinOpenAPIKey 悠悠有品开放平台应用Key
const YoupinOpenAPIKey = "12919014"

// const YoupinOpenAPIKey = "1645231"

// YoupinOpenAPIPrivateKey 悠悠有品开放平台RSA私钥（PKCS8格式）
const YoupinOpenAPIPrivateKey = "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDIjUsLt1+IkAPhIuWm/bvWebR/nK5AE6RcyLEMqT8+gU11CSjtfKXBnGdN0OCxIvorGDgSZecug3Qy3CW79ErwQyunSLAPjXw17phZogu1Q6mQ2UKBebD/fcLdodjH8LAfg2EI1kEwpw5IMH8V4rX8yUZ4XYb+MzjSfETMbqXiOAK/svRkZZS5PjbbO+q5zDNELLokoivFCxjgH647i80OlAKcH/OHahIf9gNOb86TJJVYFGA1fHHiFHFTlWAjAkyxYDx/9z4EaEEQPsD+eKXdUbSIOVqnDxGbjm2DFJMM4MolMwHs2+2YJdmWSLwf/F1q254mUqQztIwELiTV1k/PAgMBAAECggEANYnesGSGKOdFWdtekntnI0T/Rhf2Pp3fwXNELJycCRwsqONGnUuq1mph+5iY+0DapxmCkorIshaetRsnKat4O/a6lyBk++np8F3fJwKG/J9xC32sbvMlKLeSh2c2/31GD0ub4meMJKhcPDJSIu4QZkj3OpfBO2hCMZLCLQ8W0rJnKNBiZHed0C9NQ9fjWiOqi1XI8NcTYTZZ1L/3PJ0zbjHSxEIU/w84ZUDf0YLkNBT/laojWq6b9x229JIZuOjYaXhiAxK2OYaR+UD4ltsVTC+zhfLudTTWsPBcUkR67VHhjN23PUVuR8lhXoj5tPGsqHNGswo0xDRESJJHhy7kZQKBgQDobUJgZDirbZt7F5gY41M7IMgQ/0MAu0vLGhjXjMwIjO7DVDSFNnXutN/awl5gcCaPb5ON1Rb5V++R3fo9X6R80mjK44OBWeAXxr+lu8R92WL4xL7pS27igdgfJtJt6E3ARY/JXDEu32nhj92RqsD61vMEz9+FVNP3EEwhRdg3bQKBgQDc5G7oMoKyUx5Roj8nV2ezUKdMtjHt1YkmHlI5flxiVvHTedythL3cQRwZGrkTuVCKKQGwP8+J2ovLsbq8wtEWj/3WoEhRiDM6V/ncA8v7mi9H6s2ogzHMlY1YrJ8/bsrZIxEZ1l933IIJgw8h2vUrmi30PIenD/fgb5ksNi4yqwKBgQCzbY1dXmFFLeNmnitDo1JghgkM3hI6oVx8mVPuKvpj63BzCDFXWVing6iAd6Zl6o5KEselKYiHyvPd9rA06v3Pgpt1bTfbBqfxkvPmHNMumEBIbZI4BYy/fZ97RPwT7s7/DHRY7TvmxIA3qllRF7HMs11+LH+QrZDI3SL4WLP27QKBgQDKGg8HT7+Y3MeutR3HJwdwXujTHRfNnUQgpjlg9SYdq6MSdDreX8c+kCvfJD4Vt8XiwuYSli+S12x0cCaEslKPrCr5hijkwBLu3LN1A9xMVaPQzxpfhbm4j1SFv1rstLfPt2/cDfHHPu+TOGBN/4G15RkKj58l0UxgAntIokHehQKBgAjUx3ObcLMg7whnV7pZrnzeWIZY/GumIaXQfmQu3gfVZMV1vtFFnDN6doAkidLPm9XP7QhsEXRed4UKheauYwXc3PROoOZqFypfDrsdWppms99uFNTdr30kNSh0mmvi0661KApM4Llu9vgcxHaZZxGNkyX/jIzCIFCv+qWiYa+T"

// const YoupinOpenAPIPrivateKey = "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDPLzrvW523wAhPP3dJa2BBWCmwgOtnKYzSurE995AeMl0Kx8G0kVDRFVN2aorKbnoVFaRhn3AmZKeCFcZn6+Kki6OjacuJZWCnil0BOmELQ+DCbOB/sWzG9TY4bfc0qzenqxkHdmjqKLD2GWlrBrJqLtnsWWF8Yfk8Z0LzHe/LDF4W1pXJ2N9DzE7IEPOoF43BZ+Cg4JfsBcmQRX3Ev12aLk1IRVG+JrDfwZvpxXQ/rKFKHaLuZvQSDqLE78H3bhxz+j1so2qBuzhhxqTIddM1jjsvmDkCwxHAdNEW62dPLXL24QqTaKRz88LGgJKfVeudqrpNA5wCi03YE9+wzbwZAgMBAAECggEAExobTJAFerh0c60J8B2bqG6q2lSKj66BpriyiIBzm8sgL7Mi6YVn3n0hPrbcQMV+kUTXbO5Eqm+Hj6t3CWQS14Gt5s5FZy+vAYd043z4z0908KwV50QKH85iL0H5Q7SBCz8PzdO1oNg7V5dYdDcP+lEDQ+KqRavpTS/GmRByqEVbSfqSw4rlcGYOdZCM1QbYXywA53/5cVSyeiMR3mYu8F3jR9w8WGLVz5B1tdXtHKhnzSeFJbriw7YZ+86eRXjsyha/ZgPuyjC/PmWerX+r78IbQ9jBfIdZ/WnooFzyKtrwOlvuOkqiZGNB6C/DoiBjJ+opn8RXFBqznFr9NPFi8QKBgQDhoVgzB2qEUGdxWCjZPMM7gBiNwx1DyYHo6Cz0QwDp9lPF5Ag8IuhvVjZBnJ91qeC2Qo6NA58UfeVTXzbRGc0nZGLz2VbL/5UYc9Nz5TMI5qjGY5yigu2axeaunfjD5/p0KCh2575T94C1di4rH19uPxdeED5ItQzH56Fzb22nyQKBgQDrEkp1oRjVAmyOLwabWcl7cNtRgqbSteUdGynx0nOkmUurkXGos3bkl7h+hK+077XLHRw/F8zQj4h5nXaMOoblPyWYSyY+FeCaP7Fxsb66dzpxwO3k9l/N5RhYnfwG1QF+/PrC/wYXJA8t8GZoy/QKqElquC2XysNNjRHkLIw50QKBgG1W8dXwvxaVnDtaHJmBj56y6bMxHWpvKUxDzx0jpzq5y0j3w2HZDMh/f9V13/R2OVY5lLkTSzDz/YDUgEz+cuOnCyjeZuh+17K81niwVnX2yU0ykoJSbnB1fN+b4CSXs1A88bcFgL9SDoZhWeg90NItMta7imNTkXkCb8Pv+LS5AoGAR+M+Fh8AAxRMsma7NIMO1Ms8pv63mbS6rl4CQ8vCRdIDjCJcieToNRwY9cgKG+E6zTTH0JusrRNX/ykiedvkKPNYwewpc88o8sMLtuNOmqOfoe4IUn7t6X9oJKGb01MMtBMVdNCcwjLq2XetOds1dJTmbtaISuVdOLgtgoQFFiECgYEApt+IAhmpPdxLjWQigLnJT73Bz8oMLOIxKcome7Svhz9Z1qF//PPaA2PYu5xQ3DhVuo62fwcdXC3DUVUObLCgf63jkJ9YeIreL7N6h3IS+qKOiio9B6mTjx1kCzNhDxPJpc59w27p27UnTKYa0ssiwQcDFTESNhbUc83B305y7xs="
// OpenAPIClient 悠悠有品开放平台API客户端
// 这个客户端专门用于管理悠悠有品开放平台的所有API接口
// 注意：部分接口（如求购相关）需要使用Token认证，而非OpenAPI签名认证
type OpenAPIClient struct {
	client      *Client // 底层HTTP客户端（OpenAPI签名认证）
	tokenClient *Client // Token认证客户端（用于求购等接口）
}

// NewOpenAPIClient 创建新的OpenAPI客户端
// privateKey: RSA私钥(PKCS8格式)
// appKey: 应用Key
func NewOpenAPIClient(privateKey, appKey string) (*OpenAPIClient, error) {
	client, err := NewClientWithOpenAPI(privateKey, appKey)
	if err != nil {
		return nil, fmt.Errorf("创建OpenAPI客户端失败: %w", err)
	}

	return &OpenAPIClient{
		client:      client,
		tokenClient: nil, // Token客户端需要单独设置
	}, nil
}

// NewOpenAPIClientWithToken 创建带Token认证的OpenAPI客户端
// privateKey: RSA私钥(PKCS8格式)
// appKey: 应用Key
// token: 用户Token（用于求购等接口）
func NewOpenAPIClientWithToken(privateKey, appKey, token string) (*OpenAPIClient, error) {
	// 创建OpenAPI客户端
	client, err := NewClientWithOpenAPI(privateKey, appKey)
	if err != nil {
		return nil, fmt.Errorf("创建OpenAPI客户端失败: %w", err)
	}

	// 创建Token认证客户端
	tokenClient, err := NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("创建Token客户端失败: %w", err)
	}

	return &OpenAPIClient{
		client:      client,
		tokenClient: tokenClient,
	}, nil
}

// NewOpenAPIClientWithDefaultKeys 使用硬编码密钥创建OpenAPI客户端（仅OpenAPI认证，不需要Token）
// 注意：使用硬编码的 YoupinOpenAPIKey 和 YoupinOpenAPIPrivateKey
// 这个客户端仅用于查询在售商品价格等不需要Token的接口
func NewOpenAPIClientWithDefaultKeys() (*OpenAPIClient, error) {
	return NewOpenAPIClient(YoupinOpenAPIPrivateKey, YoupinOpenAPIKey)
}

// NewOpenAPIClientWithKeys 使用自定义密钥创建OpenAPI客户端
func NewOpenAPIClientWithKeys(youpinOpenAPIPrivateKey, youpinOpenAPIKey string) (*OpenAPIClient, error) {
	return NewOpenAPIClient(youpinOpenAPIPrivateKey, youpinOpenAPIKey)
}

// NewOpenAPIClientWithDefaultKeysAndToken 使用硬编码密钥创建带Token认证的OpenAPI客户端
// token: 用户Token（用于求购等接口）
// 注意：使用硬编码的 YoupinOpenAPIKey 和 YoupinOpenAPIPrivateKey
func NewOpenAPIClientWithDefaultKeysAndToken(token string) (*OpenAPIClient, error) {
	return NewOpenAPIClientWithToken(YoupinOpenAPIPrivateKey, YoupinOpenAPIKey, token)
}

// NewOpenAPIClientWithKeysAndToken 使用自定义密钥创建带Token认证的OpenAPI客户端
func NewOpenAPIClientWithKeysAndToken(token string, proxyURL string, timeout time.Duration, youpinOpenAPIPrivateKey, youpinOpenAPIKey string) (*OpenAPIClient, error) {
	// 创建OpenAPI客户端（不需要代理）
	client, err := NewOpenAPIClient(youpinOpenAPIPrivateKey, youpinOpenAPIKey)
	if err != nil {
		return nil, fmt.Errorf("创建OpenAPI客户端失败: %w", err)
	}
	// 创建支持代理的Token认证客户端
	tokenClient, err := NewClientWithTokenAndProxy(token, proxyURL, timeout)
	if err != nil {
		return nil, fmt.Errorf("创建代理Token客户端失败: %w", err)
	}

	client.tokenClient = tokenClient
	return client, nil
}

// NewOpenAPIClientWithDefaultKeysAndTokenAndProxy 使用硬编码密钥创建带Token认证和代理支持的OpenAPI客户端
// token: 用户Token（用于求购等接口）
// proxyURL: 代理地址，格式: http://username:password@host:port
// timeout: 请求超时时间
// 注意：使用硬编码的 YoupinOpenAPIKey 和 YoupinOpenAPIPrivateKey
func NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(token string, proxyURL string, timeout time.Duration) (*OpenAPIClient, error) {
	// 创建OpenAPI客户端（不需要代理）
	client, err := NewOpenAPIClient(YoupinOpenAPIPrivateKey, YoupinOpenAPIKey)
	if err != nil {
		return nil, fmt.Errorf("创建OpenAPI客户端失败: %w", err)
	}

	// 创建支持代理的Token认证客户端
	tokenClient, err := NewClientWithTokenAndProxy(token, proxyURL, timeout)
	if err != nil {
		return nil, fmt.Errorf("创建代理Token客户端失败: %w", err)
	}

	client.tokenClient = tokenClient
	return client, nil
}

// SetTokenClient 设置Token认证客户端（用于求购等接口）
func (c *OpenAPIClient) SetTokenClient(token string) error {
	tokenClient, err := NewClient(token)
	if err != nil {
		return fmt.Errorf("创建Token客户端失败: %w", err)
	}
	c.tokenClient = tokenClient
	return nil
}

// SetTokenClientWithProxy 设置支持代理的Token认证客户端
func (c *OpenAPIClient) SetTokenClientWithProxy(token string, proxyURL string, timeout time.Duration) error {
	tokenClient, err := NewClientWithTokenAndProxy(token, proxyURL, timeout)
	if err != nil {
		return fmt.Errorf("创建代理Token客户端失败: %w", err)
	}
	c.tokenClient = tokenClient
	return nil
}

// ===========================
// 查询商品列表相关接口
// ===========================

// GoodsQueryRequest 查询商品列表请求参数
type GoodsQueryRequest struct {
	TemplateID          *int    `json:"templateId,omitempty"`          // 商品模版ID（与templateHashName二选一）
	TemplateHashName    *string `json:"templateHashName,omitempty"`    // 模板hashname（与templateId二选一）
	PageSize            *int    `json:"pageSize,omitempty"`            // 每页查询数量，默认50，最大200
	Page                *int    `json:"page,omitempty"`                // 页码，最大50
	AbradeStartInterval *string `json:"abradeStartInterval,omitempty"` // 最小磨损度
	AbradeEndInterval   *string `json:"abradeEndInterval,omitempty"`   // 最大磨损度
	DopplerProperty     *int    `json:"dopplerProperty,omitempty"`     // 多普勒属性：1:P1, 2:P2, 3:P3, 4:P4, 5:绿宝石, 6:红宝石, 7:蓝宝石, 8:黑珍珠
	FadeRangeMin        *int    `json:"fadeRangeMin,omitempty"`        // 渐变区间最小值（%）
	FadeRangeMax        *int    `json:"fadeRangeMax,omitempty"`        // 渐变区间最大值（%）
	HardeningProperty   *int    `json:"hardeningProperty,omitempty"`   // 淬火属性：101:T1、102:T2、103:T3、104:T4、105：单面全蓝
	SortType            *int    `json:"sortType,omitempty"`            // 排序方式：0：更新时间倒序； 1：价格升序； 2：价格降序
}

// GoodsQueryResponse 查询商品列表响应
type GoodsQueryResponse struct {
	Code      int         `json:"code"`
	Msg       string      `json:"msg"`
	Timestamp int64       `json:"timestamp"`
	Data      []GoodsItem `json:"data"`
}

// GoodsItem 商品信息
type GoodsItem struct {
	ID                   int           `json:"id"`                   // 商品id
	TemplateID           int           `json:"templateId"`           // 商品模板id
	CommodityName        string        `json:"commodityName"`        // 商品名称
	CommodityPrice       string        `json:"commodityPrice"`       // 商品价格（单位元）
	CommodityAbrade      string        `json:"commodityAbrade"`      // 商品磨损度
	CommodityPaintSeed   int           `json:"commodityPaintSeed"`   // 图案模板
	CommodityPaintIndex  int           `json:"commodityPaintIndex"`  // 皮肤编号
	CommodityHaveNameTag int           `json:"commodityHaveNameTag"` // 是否有名称标签：0否1是
	CommodityHaveBuZhang int           `json:"commodityHaveBuZhang"` // 是否有布章：0否1是
	CommodityHaveSticker int           `json:"commodityHaveSticker"` // 是否有印花：0否1是
	ShippingMode         int           `json:"shippingMode"`         // 发货模式：0,卖家直发；1,极速发货
	TemplateIsFade       int           `json:"templateIsFade"`       // 是否渐变色：0否1是
	TemplateIsHardened   int           `json:"templateIsHardened"`   // 是否表面淬火：0否1是
	TemplateIsDoppler    int           `json:"templateIsDoppler"`    // 是否多普勒：0否1是
	CommodityStickers    []StickerInfo `json:"commodityStickers"`    // 印花数据
	CommodityDoppler     *DopplerInfo  `json:"commodityDoppler"`     // 多普勒属性
	CommodityFade        *FadeInfo     `json:"commodityFade"`        // 渐变色属性
	CommodityHardened    *HardenedInfo `json:"commodityHardened"`    // 表面淬火属性
}

// StickerInfo 印花信息
type StickerInfo struct {
	StickerID int    `json:"stickerId"` // 印花Id
	RawIndex  int    `json:"rawIndex"`  // 插槽编号
	Name      string `json:"name"`      // 印花名称
	HashName  string `json:"hashName"`  // 唯一名称
	Material  string `json:"material"`  // 材料
	ImgURL    string `json:"imgUrl"`    // 图片链接地址
	Price     string `json:"price"`     // 印花价格(单位元)
	Abrade    string `json:"abrade"`    // 磨损值
}

// DopplerInfo 多普勒属性
type DopplerInfo struct {
	Title     string `json:"title"`     // 分类名称
	AbbrTitle string `json:"abbrTitle"` // 分类缩写
	Color     string `json:"color"`     // 显示颜色
}

// FadeInfo 渐变色属性
type FadeInfo struct {
	Title         string `json:"title"`         // 属性名称
	NumerialValue string `json:"numerialValue"` // 对应数值
	Color         string `json:"color"`         // 显示颜色
}

// HardenedInfo 表面淬火属性
type HardenedInfo struct {
	Title     string `json:"title"`     // 分类名称
	AbbrTitle string `json:"abbrTitle"` // 分类缩写
	Color     string `json:"color"`     // 显示颜色
}

// GoodsQuery 查询商品列表
// 通过此接口指定任意商品模板，接口返回指定模板下价格最低的50条商品
// URL: /open/v1/api/goodsQuery
func (c *OpenAPIClient) GoodsQuery(ctx context.Context, req *GoodsQueryRequest) (*GoodsQueryResponse, error) {
	// 验证参数
	if (req.TemplateID == nil || *req.TemplateID == 0) && (req.TemplateHashName == nil || *req.TemplateHashName == "") {
		return nil, fmt.Errorf("templateId和templateHashName必须至少传入一个")
	}

	// 构建请求数据
	data := make(map[string]interface{})
	if req.TemplateID != nil {
		data["templateId"] = *req.TemplateID
	}
	if req.TemplateHashName != nil {
		data["templateHashName"] = *req.TemplateHashName
	}
	if req.PageSize != nil {
		data["pageSize"] = *req.PageSize
	}
	if req.Page != nil {
		data["page"] = *req.Page
	}
	if req.AbradeStartInterval != nil {
		data["abradeStartInterval"] = *req.AbradeStartInterval
	}
	if req.AbradeEndInterval != nil {
		data["abradeEndInterval"] = *req.AbradeEndInterval
	}
	if req.DopplerProperty != nil {
		data["dopplerProperty"] = *req.DopplerProperty
	}
	if req.FadeRangeMin != nil {
		data["fadeRangeMin"] = *req.FadeRangeMin
	}
	if req.FadeRangeMax != nil {
		data["fadeRangeMax"] = *req.FadeRangeMax
	}
	if req.HardeningProperty != nil {
		data["hardeningProperty"] = *req.HardeningProperty
	}
	if req.SortType != nil {
		data["sortType"] = *req.SortType
	}

	var response GoodsQueryResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/goodsQuery", data, &response)
	if err != nil {
		return nil, fmt.Errorf("查询商品列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// ===========================
// 批量查询在售商品价格接口
// ===========================

// BatchPriceQueryItem 批量价格查询项
type BatchPriceQueryItem struct {
	TemplateID       *int    `json:"templateId,omitempty"`       // 模板id（与templateHashName二选一）
	TemplateHashName *string `json:"templateHashName,omitempty"` // 模板hashName（与templateId二选一）
}

// BatchGetOnSaleCommodityInfo 批量查询在售商品价格
// 允许开发者通过此接口查询指定多个模板下的当前在售商品数量及在售最低价
// URL: /open/v1/api/batchGetOnSaleCommodityInfo
func (c *OpenAPIClient) BatchGetOnSaleCommodityInfo(ctx context.Context, requestList []BatchPriceQueryItem) (*BatchGetOnSaleCommodityInfoResponse, error) {
	// 验证请求列表
	if len(requestList) == 0 {
		return nil, fmt.Errorf("请求列表不能为空")
	}
	if len(requestList) > 200 {
		return nil, fmt.Errorf("请求列表数量不能超过200")
	}

	// 验证每个请求项
	for i, item := range requestList {
		if (item.TemplateID == nil || *item.TemplateID == 0) && (item.TemplateHashName == nil || *item.TemplateHashName == "") {
			return nil, fmt.Errorf("请求列表第%d项：templateId和templateHashName必须至少传入一个", i+1)
		}
	}

	// 构建请求数据
	requestListData := make([]map[string]interface{}, len(requestList))
	for i, item := range requestList {
		requestListData[i] = make(map[string]interface{})
		if item.TemplateID != nil {
			requestListData[i]["templateId"] = *item.TemplateID
		}
		if item.TemplateHashName != nil {
			requestListData[i]["templateHashName"] = *item.TemplateHashName
		}
	}

	data := map[string]interface{}{
		"requestList": requestListData,
	}

	var response BatchGetOnSaleCommodityInfoResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/batchGetOnSaleCommodityInfo", data, &response)
	if err != nil {
		return nil, fmt.Errorf("批量查询在售商品价格失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// ===========================
// 批量查询在售商品详情接口
// ===========================

// QueryTemplateSaleByCategoryRequest 批量查询在售商品详情请求参数
type QueryTemplateSaleByCategoryRequest struct {
	TypeID            *int    `json:"typeId,omitempty"`           // 武器大类ID（与typeHashName二选一）
	TypeHashName      *string `json:"typeHashName,omitempty"`     // 武器大类hashName（与typeId二选一）
	WeaponID          *string `json:"weaponId,omitempty"`         // 武器类型ID（与weaponHashName二选一）
	WeaponHashName    *string `json:"weaponHashName,omitempty"`   // 武器类型HashName（与weaponId二选一）
	PriceRangeMinimum *string `json:"priceRangeMinmun,omitempty"` // 商品价格区间最小值（单位：元）
	PriceRangeMaximum *string `json:"priceRangeMaxmun,omitempty"` // 商品价格区间最大值（单位：元）
	Page              *int    `json:"page,omitempty"`             // 页码，最大50
	PageSize          *int    `json:"pageSize,omitempty"`         // 每页显示数量，最大值200
}

// QueryTemplateSaleByCategoryResponse 批量查询在售商品详情响应
type QueryTemplateSaleByCategoryResponse struct {
	Code      int                        `json:"code"`
	Msg       string                     `json:"msg"`
	Timestamp int64                      `json:"timestamp"`
	Data      TemplateSaleByCategoryData `json:"data"`
}

// TemplateSaleByCategoryData 按分类查询商品数据
type TemplateSaleByCategoryData struct {
	CurrentPage                        int                                  `json:"currentPage"`                        // 当前页
	NewPageIsHaveContent               bool                                 `json:"newPageIsHaveContent"`               // 下一页是否还有内容
	SaleTemplateByCategoryResponseList []SaleTemplateByCategoryResponseItem `json:"saleTemplateByCategoryResponseList"` // 商品列表
}

// SaleTemplateByCategoryResponseItem 按分类查询商品项
type SaleTemplateByCategoryResponseItem struct {
	TemplateID               int    `json:"templateId"`               // 模板id
	TemplateHashName         string `json:"templateHashName"`         // 模板hash name
	TemplateName             string `json:"templateName"`             // 模版名称
	IconURL                  string `json:"iconUrl"`                  // 模板图片
	ExteriorName             string `json:"exteriorName"`             // 外观名称
	RarityName               string `json:"rarityName"`               // 品质
	TypeID                   int    `json:"typeId"`                   // 武器大类
	TypeHashName             string `json:"typeHashName"`             // 武器大类hashName
	WeaponID                 int    `json:"weaponId"`                 // 武器类型ID
	WeaponHashName           string `json:"weaponHashName"`           // 武器类型标签hashName
	MinSellPrice             string `json:"minSellPrice"`             // 在售最低价(单位元)
	FastShippingMinSellPrice string `json:"fastShippingMinSellPrice"` // 极速发货在售最低价(单位：元)
	ReferencePrice           string `json:"referencePrice"`           // 模板参考价(单位：元)
	SellNum                  int    `json:"sellNum"`                  // 在售数量
}

// QueryTemplateSaleByCategory 批量查询在售商品详情
// 允许开发者根据饰品类型或武器类型批量查询当前在售商品详情
// URL: /open/v1/api/queryTemplateSaleByCategory
func (c *OpenAPIClient) QueryTemplateSaleByCategory(ctx context.Context, req *QueryTemplateSaleByCategoryRequest) (*QueryTemplateSaleByCategoryResponse, error) {
	// 验证参数：武器大类和武器类型至少要有一个
	hasType := (req.TypeID != nil && *req.TypeID != 0) || (req.TypeHashName != nil && *req.TypeHashName != "")
	hasWeapon := (req.WeaponID != nil && *req.WeaponID != "") || (req.WeaponHashName != nil && *req.WeaponHashName != "")

	if !hasType && !hasWeapon {
		return nil, fmt.Errorf("武器大类和武器类型至少需要传入一个")
	}

	// 构建请求数据
	data := make(map[string]interface{})
	if req.TypeID != nil {
		data["typeId"] = *req.TypeID
	}
	if req.TypeHashName != nil {
		data["typeHashName"] = *req.TypeHashName
	}
	if req.WeaponID != nil {
		data["weaponId"] = *req.WeaponID
	}
	if req.WeaponHashName != nil {
		data["weaponHashName"] = *req.WeaponHashName
	}
	if req.PriceRangeMinimum != nil {
		data["priceRangeMinmun"] = *req.PriceRangeMinimum
	}
	if req.PriceRangeMaximum != nil {
		data["priceRangeMaxmun"] = *req.PriceRangeMaximum
	}
	if req.Page != nil {
		data["page"] = *req.Page
	}
	if req.PageSize != nil {
		data["pageSize"] = *req.PageSize
	}

	var response QueryTemplateSaleByCategoryResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/queryTemplateSaleByCategory", data, &response)
	if err != nil {
		return nil, fmt.Errorf("批量查询在售商品详情失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// ===========================
// 求购相关接口（使用Token认证）
// ===========================

// GetTemplatePurchaseInfo 获取物品求购信息
// 查询指定模板的求购配置信息，包括价格区间、数量限制等
// 注意：此接口使用Token认证
func (c *OpenAPIClient) GetTemplatePurchaseInfo(ctx context.Context, templateId string) (*GetTemplatePurchaseInfoResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	data := map[string]interface{}{
		"templateId": templateId,
	}

	var response GetTemplatePurchaseInfoResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseInfo", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取物品求购信息失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// PrePurchaseOrderCheck 预检查求购订单
// 在创建求购订单前进行参数验证和价格检查
// 注意：此接口使用Token认证
func (c *OpenAPIClient) PrePurchaseOrderCheck(ctx context.Context, req *PrePurchaseOrderCheckRequest) (*PrePurchaseOrderCheckResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	var response PrePurchaseOrderCheckResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/prePurchaseOrderCheck", req, &response)
	if err != nil {
		return nil, fmt.Errorf("预检查求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// SavePurchaseOrder 创建求购订单
// 创建新的求购订单，系统会自动匹配符合条件的商品
// 注意：此接口使用Token认证
func (c *OpenAPIClient) SavePurchaseOrder(ctx context.Context, req *SavePurchaseOrderRequest) (*SavePurchaseOrderResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	var response SavePurchaseOrderResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/savePurchaseOrder", req, &response)
	if err != nil {
		return nil, fmt.Errorf("创建求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// GetTemplatePurchaseOrderList 获取物品求购列表
// 查询指定模板下所有用户的求购订单列表
// 注意：此接口使用Token认证
func (c *OpenAPIClient) GetTemplatePurchaseOrderList(ctx context.Context, req *GetTemplatePurchaseOrderListRequest) (*GetTemplatePurchaseOrderListResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	var response GetTemplatePurchaseOrderListResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getTemplatePurchaseOrderList", req, &response)
	if err != nil {
		return nil, fmt.Errorf("获取物品求购列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// SearchPurchaseOrderList 搜索当前账号的求购列表
// 查询当前登录用户自己的求购订单列表
// 注意：此接口使用Token认证
func (c *OpenAPIClient) SearchPurchaseOrderList(ctx context.Context, req *SearchPurchaseOrderListRequest) (*SearchPurchaseOrderListResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	var response SearchPurchaseOrderListResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/searchPurchaseOrderList", req, &response)
	if err != nil {
		return nil, fmt.Errorf("搜索求购订单列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// GetPurchaseOrderDetail 获取求购订单详情
// 查询指定求购订单的详细信息
// 注意：此接口使用Token认证
func (c *OpenAPIClient) GetPurchaseOrderDetail(ctx context.Context, orderNo string) (*GetPurchaseOrderDetailResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	data := map[string]interface{}{
		"orderNo": orderNo,
	}

	var response GetPurchaseOrderDetailResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getPurchaseOrderDetail", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购订单详情失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// UpdatePurchaseOrder 修改求购订单
// 修改已创建的求购订单的价格或数量
// 注意：此接口使用Token认证
func (c *OpenAPIClient) UpdatePurchaseOrder(ctx context.Context, req *UpdatePurchaseOrderRequest) (*UpdatePurchaseOrderResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	var response UpdatePurchaseOrderResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/updatePurchaseOrder", req, &response)
	if err != nil {
		return nil, fmt.Errorf("修改求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// DeletePurchaseOrder 删除求购订单
// 删除一个或多个求购订单
// 注意：此接口使用Token认证
func (c *OpenAPIClient) DeletePurchaseOrder(ctx context.Context, orderNoList []string, sessionId string) (*DeletePurchaseOrderResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	data := map[string]interface{}{
		"orderNoList": orderNoList,
		"Sessionid":   sessionId,
	}

	var response DeletePurchaseOrderResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/deletePurchaseOrder", data, &response)
	if err != nil {
		return nil, fmt.Errorf("删除求购订单失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// GetPurchaseSupplyOrderList 获取求购供应订单列表
// 查询某个求购订单收到的供货订单列表
// 注意：此接口使用Token认证
func (c *OpenAPIClient) GetPurchaseSupplyOrderList(ctx context.Context, purchaseNo string) (*GetPurchaseSupplyOrderListResponse, error) {
	if c.tokenClient == nil {
		return nil, fmt.Errorf("未设置Token客户端，请先调用SetTokenClient或使用NewOpenAPIClientWithToken创建客户端")
	}

	data := map[string]interface{}{
		"purchaseNo": purchaseNo,
	}

	var response GetPurchaseSupplyOrderListResponse
	err := c.tokenClient.makeRequest(ctx, "POST", "/api/youpin/bff/trade/purchase/order/getPurchaseSupplyOrderList", data, &response)
	if err != nil {
		return nil, fmt.Errorf("获取求购供应订单列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// ===========================
// 出售相关接口（使用OpenAPI签名认证）
// ===========================

// GetUserSteamInventoryData 读取Steam库存
// 允许开发者通过此接口读取已绑定的Steam库存
// URL: /open/v1/api/getUserSteamInventoryData
// 频率：普通开发者1次/30s；白名单开发者1次/10s
func (c *OpenAPIClient) GetUserSteamInventoryData(ctx context.Context, steamId string) (*GetUserSteamInventoryDataResponse, error) {
	if steamId == "" {
		return nil, fmt.Errorf("steamId不能为空")
	}

	data := map[string]interface{}{
		"steamId": steamId,
	}

	var response GetUserSteamInventoryDataResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/getUserSteamInventoryData", data, &response)
	if err != nil {
		return nil, fmt.Errorf("读取Steam库存失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// OnShelfCommodity 上架库存饰品
// 允许开发者通过此接口批量上架库存饰品
// URL: /open/v1/api/onShelfCommodity
// 频率：普通开发者1次/5s；白名单开发者1次/s
func (c *OpenAPIClient) OnShelfCommodity(ctx context.Context, steamId string, requestList []OnShelfCommodityRequest) (*OnShelfCommodityResponse, error) {
	// 验证参数
	if steamId == "" {
		return nil, fmt.Errorf("steamId不能为空")
	}
	if len(requestList) == 0 {
		return nil, fmt.Errorf("请求列表不能为空")
	}
	if len(requestList) > 200 {
		return nil, fmt.Errorf("请求列表数量不能超过200")
	}

	// 验证每个请求项
	for i, item := range requestList {
		if item.ItemAssetID == "" {
			return nil, fmt.Errorf("请求列表第%d项：itemAssetId不能为空", i+1)
		}
		if item.Price == "" {
			return nil, fmt.Errorf("请求列表第%d项：price不能为空", i+1)
		}
	}

	// 构建请求数据
	data := map[string]interface{}{
		"steamId":     steamId,
		"requestList": requestList,
	}

	var response OnShelfCommodityResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/onShelfCommodity", data, &response)
	if err != nil {
		return nil, fmt.Errorf("上架库存饰品失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// GetUserOnSaleCommodityData 查询在售列表
// 允许开发者通过此接口查询用户的在售列表
// URL: /open/v1/api/getUserOnSaleCommodityData
// 频率：普通开发者1次/s；白名单开发者3次/s
func (c *OpenAPIClient) GetUserOnSaleCommodityData(ctx context.Context, steamId string, page, pageSize *int) (*GetUserOnSaleCommodityDataResponse, error) {
	if steamId == "" {
		return nil, fmt.Errorf("steamId不能为空")
	}

	// 构建请求数据
	data := map[string]interface{}{
		"steamId": steamId,
	}
	if page != nil {
		data["page"] = *page
	}
	if pageSize != nil {
		data["pageSize"] = *pageSize
	}

	var response GetUserOnSaleCommodityDataResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/getUserOnSaleCommodityData", data, &response)
	if err != nil {
		return nil, fmt.Errorf("查询在售列表失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// CommodityChangePrice 改价在售饰品
// 允许开发者通过此接口批量修改在售饰品的价格
// URL: /open/v1/api/commodityChangePrice
// 频率：普通开发者1次/5s；白名单开发者1次/s
func (c *OpenAPIClient) CommodityChangePrice(ctx context.Context, steamId string, requestList []CommodityChangePriceRequest) (*CommodityChangePriceResponse, error) {
	// 验证参数
	if steamId == "" {
		return nil, fmt.Errorf("steamId不能为空")
	}
	if len(requestList) == 0 {
		return nil, fmt.Errorf("请求列表不能为空")
	}
	if len(requestList) > 200 {
		return nil, fmt.Errorf("请求列表数量不能超过200")
	}

	// 验证每个请求项
	for i, item := range requestList {
		if item.CommodityID == 0 {
			return nil, fmt.Errorf("请求列表第%d项：commodityId不能为空", i+1)
		}
		if item.Price == "" {
			return nil, fmt.Errorf("请求列表第%d项：price不能为空", i+1)
		}
	}

	// 构建请求数据
	data := map[string]interface{}{
		"steamId":     steamId,
		"requestList": requestList,
	}

	var response CommodityChangePriceResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/commodityChangePrice", data, &response)
	if err != nil {
		return nil, fmt.Errorf("改价在售饰品失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}

// OffShelfCommodity 下架在售饰品
// 允许开发者通过此接口批量下架在售中的商品
// URL: /open/v1/api/offShelfCommodity
// 频率：普通开发者1次/5s；白名单开发者1次/s
func (c *OpenAPIClient) OffShelfCommodity(ctx context.Context, commodityIds []int) (*OffShelfCommodityResponse, error) {
	// 验证参数
	if len(commodityIds) == 0 {
		return nil, fmt.Errorf("商品ID列表不能为空")
	}
	if len(commodityIds) > 200 {
		return nil, fmt.Errorf("商品ID列表数量不能超过200")
	}

	// 构建请求数据
	data := map[string]interface{}{
		"commodityIds": commodityIds,
	}

	var response OffShelfCommodityResponse
	err := c.client.makeRequest(ctx, "POST", "/open/v1/api/offShelfCommodity", data, &response)
	if err != nil {
		return nil, fmt.Errorf("下架在售饰品失败: %w", err)
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s", response.Msg)
	}

	return &response, nil
}
