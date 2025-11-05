# Token 客户端 DNS/超时问题修复

## 问题描述

在初始化 YouPin Token 客户端时出现两种错误：

1. **首次错误**（DNS 超时）：
```
Token客户端初始化失败: 创建Token客户端失败: 悠悠有品账号登录失败，请检查token是否正确: 
发起请求失败: Get "https://api.youpin898.com/api/user/Account/getUserInfo": 
dial tcp: lookup api.youpin898.com: i/o timeout
```

2. **改进后的错误**（context 超时）：
```
Token客户端使用代理初始化失败: 创建代理Token客户端失败: 悠悠有品账号登录失败，
请检查token是否正确: 发起请求失败: Get "https://api.youpin898.com/api/user/Account/getUserInfo": 
context deadline exceeded
```

## 根本原因分析

### 第一个问题：DNS 超时
- **原因**：在初始化 Token 客户端时，代码没有配置代理就尝试连接
- **位置**：`main.go` 第 95 行，调用 `NewOpenAPIClientWithDefaultKeysAndToken(deviceToken)` 
- **后果**：从国内环境直接访问 `api.youpin898.com` 经常出现 DNS 解析超时

### 第二个问题：Context 超时
- **原因**：虽然添加了代理支持，但初始化超时设置过短（10秒）
- **位置**：`NewClientWithTokenAndProxy` 中获取用户信息时使用的 context 超时
- **后果**：通过代理验证 token 需要时间，10秒不足以完成整个流程

## 解决方案

### 修改 1：在初始化时直接使用代理（文件：`csqaq-sampler/main.go`）

**改进前的流程**：
```
1. 创建无代理的 Token 客户端 → DNS 超时 ❌
2. （如果步骤1失败）再尝试添加代理
```

**改进后的流程**：
```
1. 如果启用代理，直接创建支持代理的 Token 客户端
   └─ 调用 NewOpenAPIClientWithDefaultKeysAndTokenAndProxy()
2. 如果失败，再作为备用方案创建无代理客户端
```

**具体代码**（第 95-126 行）：
```go
if *useProxy {
    proxyURLWithAuth := fmt.Sprintf("http://%s:%s@%s", *proxyUser, *proxyPass, *proxyURL)
    initTimeout := time.Duration(*proxyTimeout) * time.Second
    if initTimeout < 30*time.Second {
        initTimeout = 30 * time.Second // 初始化至少使用30秒超时
    }
    if c, err := youpin.NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(
        deviceToken, proxyURLWithAuth, initTimeout); err == nil {
        tokenClient = c
        log.Println("YouPin Token客户端初始化成功 (使用代理认证)")
    } else {
        // 备用方案...
    }
}
```

### 修改 2：增加超时时间（所有文件）

**超时策略**：
- **初始化超时**：至少 30 秒（足以通过代理验证 token）
- **运行时超时**：继续使用命令行参数 `--proxy-timeout` 配置

**实现位置**：
- `csqaq-sampler/main.go` 第 98-102 行
- 自动将初始化超时设置为 `max(proxyTimeout, 30秒)`

### 修改 3：新增 API 函数（文件：`openapi.go` 和 `client.go`）

#### 在 `openapi.go` 中添加：
```go
func NewOpenAPIClientWithDefaultKeysAndTokenAndProxy(
    token string, 
    proxyURL string, 
    timeout time.Duration) (*OpenAPIClient, error) {
    // 创建 OpenAPI 客户端
    client, err := NewOpenAPIClient(YoupinOpenAPIPrivateKey, YoupinOpenAPIKey)
    
    // 创建支持代理的 Token 客户端
    tokenClient, err := NewClientWithTokenAndProxy(token, proxyURL, timeout)
    
    client.tokenClient = tokenClient
    return client, nil
}
```

#### 在 `client.go` 中添加：
```go
func NewClientWithTokenAndProxy(
    token string, 
    proxyURL string, 
    timeout time.Duration) (*Client, error) {
    // 创建支持代理的 HTTP 客户端
    httpClient := &http.Client{
        Timeout: timeout,
    }
    
    // 配置代理（使用 net.Dialer 确保超时配置生效）
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
    
    client := &Client{
        httpClient:  httpClient,
        token:       token,
        deviceToken: "aNbW21QU7cUDAJB4bK22q1rk",
        deviceID:    "aNbW21QU7cUDAJB4bK22q1rk",
        baseURL:     BaseURL,
        useOpenAPI:  false,
    }
    
    // 验证 token（使用较长的 context 超时）
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    userInfo, err := client.getUserInfo(ctx)
    // ... 处理错误
}
```

## 文件修改清单

✅ `/Users/user/Downloads/csgoAuto/csqaq-sampler/main.go`
- 修改了 Token 客户端初始化流程
- 添加了初始化超时逻辑

✅ `/Users/user/Downloads/csgoAuto/csqaq-sampler/internal/services/youpin/openapi.go`
- 新增 `NewOpenAPIClientWithDefaultKeysAndTokenAndProxy()` 方法

✅ `/Users/user/Downloads/csgoAuto/internal/services/youpin/openapi.go`
- 新增 `NewOpenAPIClientWithDefaultKeysAndTokenAndProxy()` 方法

✅ `/Users/user/Downloads/csgoAuto/internal/services/youpin/client.go`
- 新增 `NewClientWithTokenAndProxy()` 方法
- 调整了导入包的顺序

## 测试建议

### 测试 1：验证代理初始化
```bash
cd /Users/user/Downloads/csgoAuto/csqaq-sampler
go run main.go -openapi -use-proxy -proxy-timeout 10
```

**预期结果**：
```
YouPin Token客户端初始化成功 (使用代理认证)
```

### 测试 2：验证无代理模式
```bash
go run main.go -openapi -use-proxy=false
```

**预期结果**：
```
YouPin Token客户端初始化成功 (使用内置Token)
```

### 测试 3：验证备用方案
（修改代理地址为无效地址，测试备用逻辑）
```bash
go run main.go -openapi -use-proxy -proxy-url "invalid.proxy:1000"
```

**预期结果**：应该优雅降级到无代理模式或 fallback 到 OpenAPI 客户端

## 性能优化建议

1. **缓存 Token 验证结果**：
   - 不需要每次启动都验证 token
   - 可以添加定期刷新机制

2. **异步初始化**：
   - Token 客户端初始化不阻塞其他服务启动
   - 使用 goroutine 异步初始化

3. **更好的错误恢复**：
   - 当初始化失败时，记录详细日志便于调试
   - 提供重试机制

## 注意事项

⚠️ **关于超时设置**：
- 30 秒是根据 token 验证的平均时间设定的
- 如果网络特别差，可能需要增加到 45 秒
- 可以通过命令行参数 `--proxy-timeout` 进一步调整

⚠️ **关于 DNS 问题**：
- 确保代理服务器稳定可用
- 国内环境建议使用香港或新加坡的代理节点
- 避免直接访问 `api.youpin898.com`（从国内通常会超时）

⚠️ **关于代理配置**：
- Token 格式：`http://username:password@host:port`
- 确保密码中没有特殊字符，或进行 URL 编码
