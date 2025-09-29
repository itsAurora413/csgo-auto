package steam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"csgo-trader/internal/models"

	"github.com/go-resty/resty/v2"
)

type SteamService struct {
	apiKey string
	client *resty.Client
}

type SteamUser struct {
	SteamID     string `json:"steamid"`
	PersonaName string `json:"personaname"`
	Avatar      string `json:"avatar"`
	AvatarFull  string `json:"avatarfull"`
}

type SteamUserResponse struct {
	Response struct {
		Players []SteamUser `json:"players"`
	} `json:"response"`
}

type SteamMarketItem struct {
	Success     bool   `json:"success"`
	LowestPrice string `json:"lowest_price"`
	Volume      string `json:"volume"`
	MedianPrice string `json:"median_price"`
}

type SteamInventoryItem struct {
	AppID          int               `json:"appid"`
	ContextID      string            `json:"contextid"`
	AssetID        string            `json:"assetid"`
	ClassID        string            `json:"classid"`
	InstanceID     string            `json:"instanceid"`
	Amount         string            `json:"amount"`
	Descriptions   []ItemDescription `json:"descriptions"`
	MarketName     string            `json:"market_name"`
	MarketHashName string            `json:"market_hash_name"`
	Name           string            `json:"name"`
	NameColor      string            `json:"name_color"`
	Type           string            `json:"type"`
	IconURL        string            `json:"icon_url"`
	Tradable       int               `json:"tradable"`
	Marketable     int               `json:"marketable"`
	Tags           []ItemTag         `json:"tags"`
}

type ItemDescription struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type ItemTag struct {
	Category              string `json:"category"`
	InternalName          string `json:"internal_name"`
	LocalizedCategoryName string `json:"localized_category_name"`
	LocalizedTagName      string `json:"localized_tag_name"`
}

func NewSteamService(apiKey string) *SteamService {
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	return &SteamService{
		apiKey: apiKey,
		client: client,
	}
}

func (s *SteamService) GetUserInfo(steamID string) (*models.User, error) {
	url := fmt.Sprintf("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s", s.apiKey, steamID)

	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	var steamResp SteamUserResponse
	if err := json.Unmarshal(resp.Body(), &steamResp); err != nil {
		return nil, err
	}

	if len(steamResp.Response.Players) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	player := steamResp.Response.Players[0]
	return &models.User{
		SteamID:  player.SteamID,
		Username: player.PersonaName,
		Avatar:   player.AvatarFull,
	}, nil
}

func (s *SteamService) GetMarketPrice(marketHashName string) (*models.Price, error) {
	encodedName := url.QueryEscape(marketHashName)
	url := fmt.Sprintf("https://steamcommunity.com/market/priceoverview/?appid=730&currency=1&market_hash_name=%s", encodedName)

	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	var marketItem SteamMarketItem
	if err := json.Unmarshal(resp.Body(), &marketItem); err != nil {
		return nil, err
	}

	if !marketItem.Success {
		return nil, fmt.Errorf("failed to get market price")
	}

	// Parse price from string (e.g., "$1.23" -> 1.23)
	priceStr := strings.TrimPrefix(marketItem.LowestPrice, "$")
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return nil, err
	}

	// Parse volume
	volumeStr := strings.ReplaceAll(marketItem.Volume, ",", "")
	volume, _ := strconv.Atoi(volumeStr)

	return &models.Price{
		Platform:  "steam",
		Price:     price,
		Volume:    volume,
		Currency:  "USD",
		Timestamp: time.Now(),
	}, nil
}

func (s *SteamService) GetUserInventory(steamID string) ([]SteamInventoryItem, error) {
	url := fmt.Sprintf("https://steamcommunity.com/inventory/%s/730/2?l=english&count=5000", steamID)

	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	var inventoryResp struct {
		Assets       []SteamInventoryItem `json:"assets"`
		Descriptions []SteamInventoryItem `json:"descriptions"`
		Success      int                  `json:"success"`
	}

	if err := json.Unmarshal(resp.Body(), &inventoryResp); err != nil {
		return nil, err
	}

	if inventoryResp.Success != 1 {
		return nil, fmt.Errorf("failed to get inventory")
	}

	// Merge assets with descriptions
	descMap := make(map[string]SteamInventoryItem)
	for _, desc := range inventoryResp.Descriptions {
		key := desc.ClassID + "_" + desc.InstanceID
		descMap[key] = desc
	}

	var items []SteamInventoryItem
	for _, asset := range inventoryResp.Assets {
		key := asset.ClassID + "_" + asset.InstanceID
		if desc, exists := descMap[key]; exists {
			desc.AssetID = asset.AssetID
			desc.Amount = asset.Amount
			items = append(items, desc)
		}
	}

	return items, nil
}

func (s *SteamService) ValidateAPIKey() error {
	url := fmt.Sprintf("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=76561197960435530", s.apiKey)

	resp, err := s.client.R().Get(url)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("invalid API key")
	}

	return nil
}

func (s *SteamService) GetOpenIDLoginURL(returnURL string) string {
	// 解析return URL获取正确的realm
	parsedURL, err := url.Parse(returnURL)
	if err != nil {
		parsedURL = &url.URL{Scheme: "http", Host: "localhost:8080"}
	}

	realm := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	params := url.Values{}
	params.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	params.Set("openid.mode", "checkid_setup")
	params.Set("openid.return_to", returnURL)
	params.Set("openid.realm", realm)
	params.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	params.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")

	return "https://steamcommunity.com/openid/login?" + params.Encode()
}

func (s *SteamService) VerifyOpenIDResponse(params url.Values) (string, error) {
	// Validate the OpenID response
	params.Set("openid.mode", "check_authentication")

	resp, err := s.client.R().
		SetFormDataFromValues(params).
		Post("https://steamcommunity.com/openid/login")

	if err != nil {
		return "", err
	}

	if !strings.Contains(string(resp.Body()), "is_valid:true") {
		return "", fmt.Errorf("invalid OpenID response")
	}

	// Extract Steam ID from claimed_id
	claimedID := params.Get("openid.claimed_id")
	if claimedID == "" {
		return "", fmt.Errorf("no claimed_id in response")
	}

	// Steam ID is the last part of the URL
	parts := strings.Split(claimedID, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid claimed_id format")
	}

	steamID := parts[len(parts)-1]
	return steamID, nil
}
