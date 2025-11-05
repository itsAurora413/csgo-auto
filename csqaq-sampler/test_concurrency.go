package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"csqaq-sampler/internal/services/youpin"
	"github.com/joho/godotenv"
)

type TestResult struct {
	Workers          int
	TotalRequests    int
	SuccessRequests  int
	FailedRequests   int
	RateLimitErrors  int
	TimeoutErrors    int
	OtherErrors      int
	TotalDuration    time.Duration
	AvgResponseTime  time.Duration
	MinResponseTime  time.Duration
	MaxResponseTime  time.Duration
	RequestsPerSec   float64
	SuccessRate      float64
}

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using environment variables")
	}

	// Test parameters
	proxyURL := "hk.novproxy.io:1000"
	proxyUser := "qg3e2819-region-US"
	proxyPass := "mahey33h"
	templateID := 730001
	proxyTimeout := 10 * time.Second
	requestsPerWorker := 10 // 每个工作线程发送10个请求
	workerCounts := []int{2, 3, 4, 5, 6} // 测试的工作线程数

	deviceToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJmZWQ4ZGM4NTBiYWY0NDM3OWY3YzY0ZWMwNzUwYzdmZSIsIm5hbWVpZCI6IjEyOTE5MDE0IiwiSWQiOiIxMjkxOTAxNCIsInVuaXF1ZV9uYW1lIjoiWVAwMDEyOTE5MDE0IiwiTmFtZSI6IllQMDAxMjkxOTAxNCIsInZlcnNpb24iOiJRajEiLCJuYmYiOjE3NjA2Mzk3NjcsImV4cCI6MTc2MTUwMzc2NywiaXNzIjoieW91cGluODk4LmNvbSIsImRldmljZUlkIjoiZTdkZjM5ZDUtMTNmNi00NmYxLWE0MjQtMWZkNTliNTg1OTg4IiwiYXVkIjoidXNlciJ9.Gyup-6q9G6MfsFhc6Mq9PVVE0NmRR4r-6fl9PasWG6Y"

	proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", proxyUser, proxyPass, proxyURL)

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("代理并发测试 - 逐步增加工作线程")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("代理: %s\n", proxyURL)
	fmt.Printf("测试模板ID: %d\n", templateID)
	fmt.Printf("每个工作线程请求数: %d\n", requestsPerWorker)
	fmt.Println()

	var results []TestResult

	for _, workers := range workerCounts {
		fmt.Printf("🔄 测试 %d 个工作线程...\n", workers)
		fmt.Println(strings.Repeat("-", 80) + "\n")

		result := runConcurrencyTest(
			deviceToken,
			templateID,
			proxyURLWithAuth,
			proxyTimeout,
			workers,
			requestsPerWorker,
		)

		results = append(results, result)

		// Print result
		printTestResult(result)

		// Check for rate limiting
		if result.RateLimitErrors > 0 {
			fmt.Printf("⚠️  检测到限流错误 (%d)，停止测试\n", result.RateLimitErrors)
			break
		}

		// 每个测试之间等待5秒，让服务器恢复
		if workers < workerCounts[len(workerCounts)-1] {
			fmt.Println("\n等待5秒让服务器恢复...")
			time.Sleep(5 * time.Second)
		}

		fmt.Println()
	}

	// Print summary
	printSummary(results)
}

func runConcurrencyTest(
	token string,
	templateID int,
	proxyURLWithAuth string,
	timeout time.Duration,
	workers int,
	requestsPerWorker int,
) TestResult {
	result := TestResult{
		Workers:        workers,
		TotalRequests:  workers * requestsPerWorker,
		MinResponseTime: time.Hour, // Initialize to large value
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	ctx := context.Background()

	// Create clients for each worker
	clients := make([]*youpin.Client, workers)
	for i := 0; i < workers; i++ {
		client, err := youpin.NewClientWithTokenAndProxy(token, proxyURLWithAuth, timeout)
		if err != nil {
			fmt.Printf("❌ 创建客户端 %d 失败: %v\n", i, err)
			return result
		}
		clients[i] = client
	}

	// Track response times
	responseTimes := make([]time.Duration, 0, result.TotalRequests)
	var responseMu sync.Mutex

	startTime := time.Now()

	// Launch workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int, client *youpin.Client) {
			defer wg.Done()

			for r := 0; r < requestsPerWorker; r++ {
				// Add small delay between requests in same worker
				time.Sleep(100 * time.Millisecond)

				reqStart := time.Now()
				response, err := client.GetTemplatePurchaseOrderList(ctx, templateID, 1, 5)
				reqDuration := time.Since(reqStart)

				responseMu.Lock()
				responseTimes = append(responseTimes, reqDuration)
				responseMu.Unlock()

				mu.Lock()
				if err != nil {
					result.FailedRequests++

					// Categorize error
					errStr := err.Error()
					if contains(errStr, "84104") || contains(errStr, "频繁") {
						result.RateLimitErrors++
						fmt.Printf("❌ Worker %d Req %d: 限流错误 (84104)\n", workerID, r+1)
					} else if contains(errStr, "timeout") || contains(errStr, "deadline") {
						result.TimeoutErrors++
						fmt.Printf("⚠️  Worker %d Req %d: 超时\n", workerID, r+1)
					} else {
						result.OtherErrors++
						fmt.Printf("❌ Worker %d Req %d: 其他错误 - %v\n", workerID, r+1, err)
					}
				} else if response.Code != 0 {
					result.FailedRequests++
					result.OtherErrors++
					fmt.Printf("❌ Worker %d Req %d: API返回错误 - Code: %d, Msg: %s\n", workerID, r+1, response.Code, response.Msg)
				} else {
					result.SuccessRequests++
					fmt.Printf("✅ Worker %d Req %d: 成功 (响应时间: %v)\n", workerID, r+1, reqDuration)
				}
				mu.Unlock()
			}
		}(w, clients[w])
	}

	wg.Wait()
	result.TotalDuration = time.Since(startTime)

	// Calculate statistics
	if len(responseTimes) > 0 {
		var totalTime time.Duration
		for _, t := range responseTimes {
			totalTime += t
			if t < result.MinResponseTime {
				result.MinResponseTime = t
			}
			if t > result.MaxResponseTime {
				result.MaxResponseTime = t
			}
		}
		result.AvgResponseTime = totalTime / time.Duration(len(responseTimes))
	}

	result.RequestsPerSec = float64(result.TotalRequests) / result.TotalDuration.Seconds()
	result.SuccessRate = float64(result.SuccessRequests) / float64(result.TotalRequests) * 100

	return result
}

func printTestResult(result TestResult) {
	fmt.Printf("📊 结果 (%d 个工作线程):\n", result.Workers)
	fmt.Printf("   总请求数: %d\n", result.TotalRequests)
	fmt.Printf("   成功: %d (%.1f%%)\n", result.SuccessRequests, result.SuccessRate)
	fmt.Printf("   失败: %d\n", result.FailedRequests)
	if result.RateLimitErrors > 0 {
		fmt.Printf("     - 限流错误 (84104): %d ⚠️\n", result.RateLimitErrors)
	}
	if result.TimeoutErrors > 0 {
		fmt.Printf("     - 超时: %d\n", result.TimeoutErrors)
	}
	if result.OtherErrors > 0 {
		fmt.Printf("     - 其他错误: %d\n", result.OtherErrors)
	}
	fmt.Printf("   总耗时: %v\n", result.TotalDuration)
	fmt.Printf("   吞吐量: %.2f 请求/秒\n", result.RequestsPerSec)
	fmt.Printf("   响应时间:\n")
	fmt.Printf("     - 平均: %v\n", result.AvgResponseTime)
	fmt.Printf("     - 最小: %v\n", result.MinResponseTime)
	fmt.Printf("     - 最大: %v\n", result.MaxResponseTime)
}

func printSummary(results []TestResult) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("📈 汇总结果")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Println()
	fmt.Println("工作线程 | 成功率  | 吞吐量      | 平均响应 | 限流错误 | 状态")
	fmt.Println(strings.Repeat("-", 80))

	var recommendedWorkers int
	var maxSuccessRate float64

	for _, result := range results {
		status := "✅ 正常"
		if result.RateLimitErrors > 0 {
			status = "❌ 限流"
		} else if result.FailedRequests > 0 {
			status = "⚠️  有错误"
		}

		if result.SuccessRate > maxSuccessRate && result.RateLimitErrors == 0 {
			maxSuccessRate = result.SuccessRate
			recommendedWorkers = result.Workers
		}

		fmt.Printf("    %d    | %6.1f%% | %6.2f req/s | %6v | %8d | %s\n",
			result.Workers,
			result.SuccessRate,
			result.RequestsPerSec,
			result.AvgResponseTime,
			result.RateLimitErrors,
			status,
		)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("🎯 推荐配置: %d 个工作线程\n", recommendedWorkers)
	fmt.Printf("   理由: 最高成功率 (%.1f%%)，没有限流错误\n", maxSuccessRate)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Println()
	fmt.Println("📋 建议:")
	fmt.Println("1. 使用推荐的工作线程数部署")
	fmt.Println("2. 如果需要更高吞吐量，可考虑:")
	fmt.Println("   - 增加代理IP池，使用不同的账户")
	fmt.Println("   - 增加请求间隔时间")
	fmt.Println("3. 持续监控错误日志，查看是否出现新的限流")
}

func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
