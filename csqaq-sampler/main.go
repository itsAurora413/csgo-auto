package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"csqaq-sampler/internal/config"
	smodels "csqaq-sampler/internal/models"
	"csqaq-sampler/internal/services"
	"csqaq-sampler/internal/services/youpin"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Define command-line flags
	var (
		proxyURL       = flag.String("proxy-url", "hk.novproxy.io:1000", "代理服务器地址")
		proxyUser      = flag.String("proxy-user", "qg3e2819-region-US", "代理用户名")
		proxyPass      = flag.String("proxy-pass", "mahey33h", "代理密码")
		numWorkers     = flag.Int("num-workers", 3, "并发工作线程数（默认3）")
		useProxy       = flag.Bool("use-proxy", true, "是否使用代理")
		proxyTimeout   = flag.Int("proxy-timeout", 10, "代理请求超时时间（秒）")
		useMultithread = flag.Bool("multithread", false, "使用多线程采样（启用时忽略基础采样器）")
		useOpenAPI     = flag.Bool("openapi", false, "使用OpenAPI采样器（直接调用悠悠OpenAPI）")
		useDualAccount = flag.Bool("dual-account", true, "使用双账号采样（A、B两个账号并行处理）")
	)
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Create proxy configuration
	proxyConfig := &services.ProxyConfig{
		URL:          *proxyURL,
		Username:     *proxyUser,
		Password:     *proxyPass,
		Timeout:      time.Duration(*proxyTimeout) * time.Second,
		Enabled:      *useProxy,
		BindInterval: 35 * time.Second, // IP binding interval (per original CSQAQ logic)
	}

	// Initialize database
	db, err := initializeDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("CSQAQ Standalone Sampler initialized successfully")
	log.Printf("Using API Key: %s", maskAPIKey(cfg.CSQAQAPIKey))
	log.Printf("Database connected: %s", maskDSN(cfg.DatabaseURL))
	log.Printf("Proxy enabled: %v, Workers: %d", proxyConfig.Enabled, *numWorkers)
	if proxyConfig.Enabled {
		log.Printf("Proxy config: %s (user: %s)", proxyConfig.URL, maskAPIKey(proxyConfig.Username))
	}

	// Create CSQAQ API request function
	makeRequest := func(endpoint string, params map[string]string) ([]byte, error) {
		return makeCSQAQRequest(endpoint, params, cfg.CSQAQAPIKey)
	}

	// Start sampler
	var sampler interface{}

	if *useDualAccount {
		// 使用双账号采样模式
		log.Println("启用双账号采样模式 (账号A + 账号B并行处理)")

		apiKeyA := "1645231"
		apiPrivateKeyA := "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDJ6DgjnZlxJ44qIEISM2If/1ky3ubFenWQX3EDuc7kz8s9yN6qWMln8Lp44Usk7YIqL8AXrpLiNAFUFKdIXdYy8fwbEv6bowOVAygKZa9BFk8avq7pFLmbd19PzSYp596b4/NeyAV8pACU4FivlZxa46q92u799aTYPGgnUVPTxDFgAnwjzu9StT4wUVcZlWcKLWhdJaR3ZBhaRMtgW/K8T7EEK4lBmfnY3aDuuYDp26VBREfwwtRPmxi7KO9yvjvteCm6rAC0kuUrCWFnX9X55fx/o2l3VT+JC3Z8u1UHI61hRV2odnlYpeOApsRv/oHewzwcptUMnvPGop2c3+xhAgMBAAECggEAFEEHiqkXWKbJZ9frENTXOdL9cXEzUKmPbA9q8J79zm2622SQU6HK+HKJXjFpfpeVyGIYYLfKM8dYP8U/n66MG3mzWsrtwBKa/CKALITVTw/sGQh6VtbVpK8VoFV5x9fi+JvmEMK7bCyug0C7HMgDEooGmXuCIHc5FVj/8LvDjzlQRm1ElAs/qzPnKVNw3s8S1lExDuPqkPo4QZILXosAQrybJwClfg9bVMD0K5bGxf+OLiGA5Mp8BpRcoH2KYXOYVvXNNHgeKb4A7wOBOW9Zd1g11HkaoojhtGeeu472ozejV5fsnAIvf6VlpEFg/cQLCLVkCoCZk3qNGxwRCtbeMQKBgQDh8vpiqdQnNgWilVNAhuFqZUxJlZDVu64W1Cu4QDP7neKiM135A9rGnaH4D1eUJrV7z8PuEXDIQWBkwGL5Bl23B4wv5f4XMPpXzmyhJCm/ID/on81gnhzU14AF3kmb+kj7NHXnwu7LnBt2rM7zqMVIg+tL0tHVHZQ3N/OskDCVOQKBgQDkwqsXcv8k90WR/3QHeWJ4cB1k6DHBkOUuok7QYR9bho5lCKuCwo6UComXRSddYBBMyx+4kny98xe3os0/1b1Fqt+xlj8LnvvLDS1xKx1J2BMVM27DEp3z5YHHCfMLRjiqR9z5d5Js/vJnE/Tk2xTcsBNwTCS51swFTOXISwB4aQKBgQCaKltE7nNevbFimVhQcrdjDPLcUjycWI4T7XhXIxdU2wlDeqDnwwWG9w8IXg93emHhtIUO7r9xI4pNtCbTmtBejbvNArQ3xqdJOgNMf4wsaCiy5DYRclBhuRgU6PJ4hnE9VTINjCe6rSzf8FDUV5ckPz5QYevh6WiztG2ClG9RoQKBgH3Mr9nNW5IwiQmouV4C2qvwu7lqFzKjQ+VJTJ47SstCS7wz8F2QMGgpA1E0rlkjItOYQHF33TF4JWrPFKAuIk/Vj/5Q6YqyezVGod7CHvEk6hmWlyqfak1dwOh8CDQDAdZifpqRruxRp2wYBWx1LhvOmGIA/ZFVFg00JIjo9fFBAoGBANaZD1P0P/MHee8mH4d4DTl5AjKelmGpNrz1q4TmqYfJfMhL7NUQwEH2GHEUhLYiXvrizG+4/inhEdIwu/KYb6BnRUyYYvubJWcXXfvBEOg3zs9/IqfkavhNBlxhyIgCWKE64Eumx9d0qpNeQmrDrbN+9yc4IMU/RSxDsWjF3zEV"

		apiKeyB := "12919014"
		apiPrivateKeyB := "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDIjUsLt1+IkAPhIuWm/bvWebR/nK5AE6RcyLEMqT8+gU11CSjtfKXBnGdN0OCxIvorGDgSZecug3Qy3CW79ErwQyunSLAPjXw17phZogu1Q6mQ2UKBebD/fcLdodjH8LAfg2EI1kEwpw5IMH8V4rX8yUZ4XYb+MzjSfETMbqXiOAK/svRkZZS5PjbbO+q5zDNELLokoivFCxjgH647i80OlAKcH/OHahIf9gNOb86TJJVYFGA1fHHiFHFTlWAjAkyxYDx/9z4EaEEQPsD+eKXdUbSIOVqnDxGbjm2DFJMM4MolMwHs2+2YJdmWSLwf/F1q254mUqQztIwELiTV1k/PAgMBAAECggEANYnesGSGKOdFWdtekntnI0T/Rhf2Pp3fwXNELJycCRwsqONGnUuq1mph+5iY+0DapxmCkorIshaetRsnKat4O/a6lyBk++np8F3fJwKG/J9xC32sbvMlKLeSh2c2/31GD0ub4meMJKhcPDJSIu4QZkj3OpfBO2hCMZLCLQ8W0rJnKNBiZHed0C9NQ9fjWiOqi1XI8NcTYTZZ1L/3PJ0zbjHSxEIU/w84ZUDf0YLkNBT/laojWq6b9x229JIZuOjYaXhiAxK2OYaR+UD4ltsVTC+zhfLudTTWsPBcUkR67VHhjN23PUVuR8lhXoj5tPGsqHNGswo0xDRESJJHhy7kZQKBgQDobUJgZDirbZt7F5gY41M7IMgQ/0MAu0vLGhjXjMwIjO7DVDSFNnXutN/awl5gcCaPb5ON1Rb5V++R3fo9X6R80mjK44OBWeAXxr+lu8R92WL4xL7pS27igdgfJtJt6E3ARY/JXDEu32nhj92RqsD61vMEz9+FVNP3EEwhRdg3bQKBgQDc5G7oMoKyUx5Roj8nV2ezUKdMtjHt1YkmHlI5flxiVvHTedythL3cQRwZGrkTuVCKKQGwP8+J2ovLsbq8wtEWj/3WoEhRiDM6V/ncA8v7mi9H6s2ogzHMlY1YrJ8/bsrZIxEZ1l933IIJgw8h2vUrmi30PIenD/fgb5ksNi4yqwKBgQCzbY1dXmFFLeNmnitDo1JghgkM3hI6oVx8mVPuKvpj63BzCDFXWVing6iAd6Zl6o5KEselKYiHyvPd9rA06v3Pgpt1bTfbBqfxkvPmHNMumEBIbZI4BYy/fZ97RPwT7s7/DHRY7TvmxIA3qllRF7HMs11+LH+QrZDI3SL4WLP27QKBgQDKGg8HT7+Y3MeutR3HJwdwXujTHRfNnUQgpjlg9SYdq6MSdDreX8c+kCvfJD4Vt8XiwuYSli+S12x0cCaEslKPrCr5hijkwBLu3LN1A9xMVaPQzxpfhbm4j1SFv1rstLfPt2/cDfHHPu+TOGBN/4G15RkKj58l0UxgAntIokHehQKBgAjUx3ObcLMg7whnV7pZrnzeWIZY/GumIaXQfmQu3gfVZMV1vtFFnDN6doAkidLPm9XP7QhsEXRed4UKheauYwXc3PROoOZqFypfDrsdWppms99uFNTdr30kNSh0mmvi0661KApM4Llu9vgcxHaZZxGNkyX/jIzCIFCv+qWiYa+T"

		// Initialize Token clients with provided tokens
		deviceTokenA := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJlNGIxNWQ4MmZmNDE0NjE3OTAxODJjZjMxMmI4MThjMiIsIm5hbWVpZCI6IjE2NDUyMzEiLCJJZCI6IjE2NDUyMzEiLCJ1bmlxdWVfbmFtZSI6IllQMDAwMTY0NTIzMSIsIk5hbWUiOiJZUDAwMDE2NDUyMzEiLCJ2ZXJzaW9uIjoiR0RBIiwibmJmIjoxNzYzNTMyMTA4LCJleHAiOjE3NjQzOTYxMDgsImlzcyI6InlvdXBpbjg5OC5jb20iLCJkZXZpY2VJZCI6IjdiOGM5MjY3LTBkZjAtNDQ1My1iMDNkLTcyZTg0NWRkNDVjMCIsImF1ZCI6InVzZXIifQ.lAMiMd-uqHiw8iC537jZLCDr81jQnaU_RMitmSLzUwA"
		deviceTokenB := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiI5NjBiZTk3ZTM3ODA0ZDYzYTg4NWZiOTVmNTJiMWE5ZiIsIm5hbWVpZCI6IjEyOTE5MDE0IiwiSWQiOiIxMjkxOTAxNCIsInVuaXF1ZV9uYW1lIjoiWVAwMDEyOTE5MDE0IiwiTmFtZSI6IllQMDAxMjkxOTAxNCIsInZlcnNpb24iOiJNWUsiLCJuYmYiOjE3NjM1MzE3OTMsImV4cCI6MTc2NDM5NTc5MywiaXNzIjoieW91cGluODk4LmNvbSIsImRldmljZUlkIjoiZTdkZjM5ZDUtMTNmNi00NmYxLWE0MjQtMWZkNTliNTg1OTg4IiwiYXVkIjoidXNlciJ9.ZMDr3_RNA9VxilBUY11i0DWrG_lM2it1SDPKfDVweQs"

		var clientA, clientB *youpin.OpenAPIClient

		if *useProxy {
			proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", *proxyUser, *proxyPass, *proxyURL)
			initTimeout := time.Duration(*proxyTimeout) * time.Second
			if initTimeout < 30*time.Second {
				initTimeout = 30 * time.Second
			}

			// Initialize token client A
			if c, err := youpin.NewOpenAPIClientWithKeysAndToken(deviceTokenA, proxyURLWithAuth, initTimeout, apiPrivateKeyA, apiKeyA); err == nil {
				clientA = c
				log.Println("账号A - Token客户端初始化成功 (使用代理)")
			} else {
				log.Printf("账号A - Token客户端初始化失败: %v\n", err)
				return
			}

			// Initialize token client B
			if c, err := youpin.NewOpenAPIClientWithKeysAndToken(deviceTokenB, proxyURLWithAuth, initTimeout, apiPrivateKeyB, apiKeyB); err == nil {
				clientB = c
				log.Println("账号B - Token客户端初始化成功 (使用代理)")
			} else {
				log.Printf("账号B - Token客户端初始化失败: %v\n", err)
				return
			}
		} else {
			// Initialize without proxy
			if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(deviceTokenA); err == nil {
				clientA = c
				log.Println("账号A - Token客户端初始化成功 (无代理)")
			} else {
				log.Printf("账号A - Token客户端初始化失败: %v\n", err)
				return
			}

			if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(deviceTokenB); err == nil {
				clientB = c
				log.Println("账号B - Token客户端初始化成功 (无代理)")
			} else {
				log.Printf("账号B - Token客户端初始化失败: %v\n", err)
				return
			}
		}

		// Create dual account sampler
		dualSampler, err := services.NewDualAccountSampler(db, clientA, clientB, *numWorkers, proxyConfig)
		if err != nil {
			log.Fatal("Failed to create dual account sampler:", err)
		}
		dualSampler.Start()
		sampler = dualSampler

	} else if *useOpenAPI {
		// 使用OpenAPI采样器 (新方式: 直接调用悠悠OpenAPI + Token认证 + 代理)
		log.Println("启用OpenAPI采样模式 (直接调用悠悠有品OpenAPI)")

		// Initialize YouPin OpenAPI client (OpenAPI认证)
		ypClient, err := youpin.NewOpenAPIClientWithDefaultKeys()
		if err != nil {
			log.Fatal("Failed to create YouPin OpenAPI client:", err)
		}
		log.Println("YouPin OpenAPI客户端初始化成功 (OpenAPI认证)")

		// Initialize YouPin Token client (Token认证 + 代理)
		// 使用写死的 Token (生产环境使用的官方Token)
		var tokenClient *youpin.OpenAPIClient
		// deviceToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiIzMDA1ZDcyNGI3ZGU0ZGI5YjlmYThkNDdiZjM0MjkzNyIsIm5hbWVpZCI6IjEyOTE5MDE0IiwiSWQiOiIxMjkxOTAxNCIsInVuaXF1ZV9uYW1lIjoiWVAwMDEyOTE5MDE0IiwiTmFtZSI6IllQMDAxMjkxOTAxNCIsInZlcnNpb24iOiJnWVQiLCJuYmYiOjE3NjE3MDQ3OTEsImV4cCI6MTc2MjU2ODc5MSwiaXNzIjoieW91cGluODk4LmNvbSIsImRldmljZUlkIjoiZTdkZjM5ZDUtMTNmNi00NmYxLWE0MjQtMWZkNTliNTg1OTg4IiwiYXVkIjoidXNlciJ9.vfgOGpQJO7_mMjgvbVJcaPhzkf2IqMKsri2Uzi-pmmY"
		deviceToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJqdGkiOiI5MWM3ZDUyOWIwMzk0NGRiOTFlMDllYjNmMDUzMzgxZiIsIm5hbWVpZCI6IjE2NDUyMzEiLCJJZCI6IjE2NDUyMzEiLCJ1bmlxdWVfbmFtZSI6IllQMDAwMTY0NTIzMSIsIk5hbWUiOiJZUDAwMDE2NDUyMzEiLCJ2ZXJzaW9uIjoickdaIiwibmJmIjoxNzYxODI5NTE2LCJleHAiOjE3NjI2OTM1MTYsImlzcyI6InlvdXBpbjg5OC5jb20iLCJkZXZpY2VJZCI6ImU3ZGYzOWQ1LTEzZjYtNDZmMS1hNDI0LTFmZDU5YjU4NTk4OCIsImF1ZCI6InVzZXIifQ.x0jZzw6RCNcpyzA8CrB5MIJFYVzpDrKBvfmnYZVrvng"

		if *useProxy {
			// 如果启用代理，直接使用支持代理的初始化方法
			// 注意：初始化时使用更长的超时（30秒），以便有足够时间验证token
			proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", *proxyUser, *proxyPass, *proxyURL)
			initTimeout := time.Duration(*proxyTimeout) * time.Second
			if initTimeout < 30*time.Second {
				initTimeout = 30 * time.Second // 初始化至少使用30秒超时
			}
			if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(deviceToken, proxyURLWithAuth, initTimeout); err == nil {
				tokenClient = c
				log.Println("YouPin Token客户端初始化成功 (使用代理认证)")
			} else {
				log.Printf("Token客户端使用代理初始化失败: %v, 将尝试无代理模式\n", err)
				// 备用方案：先创建无代理客户端，然后添加代理
				if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(deviceToken); err == nil {
					tokenClient = c
					// 再尝试设置代理
					if err := tokenClient.SetTokenClientWithProxy(deviceToken, proxyURLWithAuth, initTimeout); err != nil {
						log.Printf("设置代理Token客户端失败: %v, 将无法获取求购价\n", err)
					} else {
						log.Println("YouPin Token客户端初始化成功 (使用代理认证)")
					}
				} else {
					log.Printf("Token客户端初始化失败: %v, 将无法获取求购价\n", err)
					tokenClient = ypClient // Fallback to OpenAPI client
				}
			}
		} else {
			// 不使用代理
			if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndToken(deviceToken); err == nil {
				tokenClient = c
				log.Println("YouPin Token客户端初始化成功 (使用内置Token)")
			} else {
				log.Printf("Token客户端初始化失败: %v, 将无法获取求购价\n", err)
				tokenClient = ypClient // Fallback to OpenAPI client
			}
		}

		// Create OpenAPI sampler (with proxy config for Token requests)
		openAPISampler, err := services.NewOpenAPISamplerV3(db, ypClient, tokenClient, *numWorkers, proxyConfig)
		if err != nil {
			log.Fatal("Failed to create OpenAPI sampler:", err)
		}
		openAPISampler.Start()
		sampler = openAPISampler

	} else if *useMultithread {
		// 使用多线程采样器 (旧方式: CSQAQ API + 代理池)
		log.Println("启用多线程采样模式 (CSQAQ API)")
		mtSampler, err := services.NewMultiThreadedSampler(db, makeRequest, proxyConfig, *numWorkers, cfg.CSQAQAPIKey)
		if err != nil {
			log.Fatal("Failed to create multithreaded sampler:", err)
		}
		mtSampler.Start()
		sampler = mtSampler
	} else {
		// 使用基础采样模式
		log.Println("使用基础采样模式 (CSQAQ API)")
		sampler = services.StartEnhancedCSQAQSampler(db, makeRequest)
	}

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Println("CSQAQ Sampler is running. Press Ctrl+C to stop...")
	<-c

	log.Println("Shutting down sampler...")
	if dualSampler, ok := sampler.(*services.DualAccountSampler); ok {
		dualSampler.Stop()
	} else if openAPISampler, ok := sampler.(*services.OpenAPISamplerV3); ok {
		openAPISampler.Stop()
	} else if mtSampler, ok := sampler.(*services.MultiThreadedSampler); ok {
		mtSampler.Stop()
	} else if basicSampler, ok := sampler.(*services.EnhancedCSQAQSampler); ok {
		basicSampler.Stop()
	}
	log.Println("Sampler stopped gracefully")
}

func initializeDatabase(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Set connection pool parameters
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database initialized successfully")

	// Migration: ensure yyyp_template_id column exists
	if err := ensureCSQAQSnapshotTemplateID(db); err != nil {
		log.Printf("Migration warning: %v", err)
	}
	return db, nil
}

// CSQAQ API constants and functions
const CSQAQ_API_BASE = "https://api.csqaq.com/api/v1/"

var lastBindTime time.Time

func ensureIPBound(apiKey string) error {
	// Bind IP every 35 seconds to avoid rate limits
	if time.Since(lastBindTime) < 35*time.Second {
		return nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := CSQAQ_API_BASE + "sys/bind_local_ip"

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("ApiToken", apiKey)
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to bind local IP, status: %d, response: %s", resp.StatusCode, string(body))
	}

	lastBindTime = time.Now()
	fmt.Printf("Successfully bound local IP to CSQAQ API\n")
	return nil
}

func makeCSQAQRequest(endpoint string, params map[string]string, apiKey string) ([]byte, error) {
	// Ensure IP is bound before making API requests
	if err := ensureIPBound(apiKey); err != nil {
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

// Utility functions
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}

func maskDSN(dsn string) string {
	if len(dsn) <= 20 {
		return "****"
	}
	return dsn[:10] + "****" + dsn[len(dsn)-10:]
}

// ensureCSQAQSnapshotTemplateID adds yyyp_template_id column to csqaq_good_snapshots if missing
func ensureCSQAQSnapshotTemplateID(db *gorm.DB) error {
	// Prefer GORM migrator checks
	if db.Migrator().HasColumn(&smodels.CSQAQGoodSnapshot{}, "yyyp_template_id") {
		return nil
	}
	// Try adding via migrator
	if err := db.Migrator().AddColumn(&smodels.CSQAQGoodSnapshot{}, "YYYPTemplateID"); err == nil {
		_ = db.Migrator().CreateIndex(&smodels.CSQAQGoodSnapshot{}, "YYYPTemplateID")
		log.Println("Added column yyyp_template_id via GORM migrator")
		return nil
	}
	// Fallback to raw SQL
	var count int64
	checkSQL := `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'csqaq_good_snapshots' AND column_name = 'yyyp_template_id'`
	if err := db.Raw(checkSQL).Scan(&count).Error; err != nil {
		return fmt.Errorf("failed checking yyyp_template_id column: %w", err)
	}
	if count > 0 {
		return nil
	}
	if err := db.Exec(`ALTER TABLE csqaq_good_snapshots ADD COLUMN yyyp_template_id BIGINT NULL`).Error; err != nil {
		return fmt.Errorf("failed adding yyyp_template_id column: %w", err)
	}
	_ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cgs_yyyp_template_id ON csqaq_good_snapshots (yyyp_template_id)`).Error
	log.Println("Added column yyyp_template_id to csqaq_good_snapshots")
	return nil
}
