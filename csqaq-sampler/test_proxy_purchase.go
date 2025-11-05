package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"csqaq-sampler/internal/services/youpin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Test parameters
	proxyURL := "hk.novproxy.io:1000"
	proxyUser := "qg3e2819-region-US"
	proxyPass := "mahey33h"
	templateID := 730001 // CS:GO商品模板ID
	proxyTimeout := 10 * time.Second

	fmt.Println("===== 测试求购接口代理连接 =====")
	fmt.Printf("代理配置: %s@%s\n", maskUser(proxyUser), proxyURL)
	fmt.Printf("测试模板ID: %d\n\n", templateID)

	// 获取内置Token
	deviceToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJmZWQ4ZGM4NTBiYWY0NDM3OWY3YzY0ZWMwNzUwYzdmZSIsIm5hbWVpZCI6IjEyOTE5MDE0IiwiSWQiOiIxMjkxOTAxNCIsInVuaXF1ZV9uYW1lIjoiWVAwMDEyOTE5MDE0IiwiTmFtZSI6IllQMDAxMjkxOTAxNCIsInZlcnNpb24iOiJRajEiLCJuYmYiOjE3NjA2Mzk3NjcsImV4cCI6MTc2MTUwMzc2NywiaXNzIjoieW91cGluODk4LmNvbSIsImRldmljZUlkIjoiZTdkZjM5ZDUtMTNmNi00NmYxLWE0MjQtMWZkNTliNTg1OTg4IiwiYXVkIjoidXNlciJ9.Gyup-6q9G6MfsFhc6Mq9PVVE0NmRR4r-6fl9PasWG6Y"

	// Test 1: 无代理请求
	fmt.Println("📌 测试1: 无代理直接请求求购接口...")
	testDirectRequest(deviceToken, templateID)

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Test 2: 使用代理请求
	fmt.Println("📌 测试2: 使用代理请求求购接口...")
	proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", proxyUser, proxyPass, proxyURL)
	testProxyRequest(deviceToken, templateID, proxyURLWithAuth, proxyTimeout)

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Test 3: 连续多次请求（测试是否会被限制）
	fmt.Println("📌 测试3: 连续5次代理请求（测试速率限制）...")
	for i := 1; i <= 5; i++ {
		fmt.Printf("\n第 %d 次请求:\n", i)
		testProxyRequest(deviceToken, templateID, proxyURLWithAuth, proxyTimeout)
		if i < 5 {
			time.Sleep(2 * time.Second) // 每次间隔2秒
		}
	}
}

func testDirectRequest(token string, templateID int) {
	ctx := context.Background()

	// Create client without proxy
	client, err := youpin.NewClientWithToken(token)
	if err != nil {
		fmt.Printf("❌ 创建客户端失败: %v\n", err)
		return
	}

	// Make request to GetTemplatePurchaseOrderList
	response, err := client.GetTemplatePurchaseOrderList(ctx, templateID, 1, 5)
	if err != nil {
		fmt.Printf("❌ 直接请求失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 直接请求成功\n")
	fmt.Printf("   响应码: %d\n", response.Code)
	fmt.Printf("   消息: %s\n", response.Msg)
	fmt.Printf("   求购列表数量: %d\n", len(response.Data))

	// Print first purchase order if exists
	if len(response.Data) > 0 {
		po := response.Data[0]
		fmt.Printf("   第一个求购订单: 价格=%.2f\n", po.PurchasePrice)
	}
}

func testProxyRequest(token string, templateID int, proxyURLWithAuth string, timeout time.Duration) {
	ctx := context.Background()

	// Create client with proxy
	client, err := youpin.NewClientWithTokenAndProxy(token, proxyURLWithAuth, timeout)
	if err != nil {
		fmt.Printf("❌ 创建代理客户端失败: %v\n", err)
		return
	}

	// Make request to GetTemplatePurchaseOrderList
	startTime := time.Now()
	response, err := client.GetTemplatePurchaseOrderList(ctx, templateID, 1, 5)
	elapsed := time.Since(startTime)

	if err != nil {
		fmt.Printf("❌ 代理请求失败 (耗时: %v): %v\n", elapsed, err)

		// 检查是否被封禁
		if isBlockedError(err) {
			fmt.Println("   ⚠️  警告: 可能被悠悠有品封禁!")
		}
		return
	}

	fmt.Printf("✅ 代理请求成功 (耗时: %v)\n", elapsed)
	fmt.Printf("   响应码: %d\n", response.Code)
	fmt.Printf("   消息: %s\n", response.Msg)
	fmt.Printf("   求购列表数量: %d\n", len(response.Data))

	// Print first purchase order if exists
	if len(response.Data) > 0 {
		po := response.Data[0]
		fmt.Printf("   第一个求购订单: 价格=%.2f\n", po.PurchasePrice)
	}
}

func isBlockedError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for common blocking indicators
	blockingPatterns := []string{
		"429",             // HTTP 429 Too Many Requests
		"403",             // HTTP 403 Forbidden
		"84104",           // YouPin API rate limit
		"您的操作太频繁",      // "Your operation is too frequent"
		"被封禁",            // "Blocked"
		"IP受限",           // "IP restricted"
		"Connection refused", // Connection issues
	}

	for _, pattern := range blockingPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func maskUser(user string) string {
	if len(user) <= 4 {
		return "****"
	}
	return user[:2] + "***" + user[len(user)-2:]
}
