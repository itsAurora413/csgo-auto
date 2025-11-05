# 修改前后对比

## 问题演示

### ❌ 修改前（错误的 - 会被 Go 规范化）

日志输出中的请求头：
```
Devicetoken: aNbW21QU7cUDAJB4bK22q1rk           # ❌ 错误！应该是 DeviceToken
Requesttag: A6GEMCX3UDOE9KW3QNSS0FTI48CYFUQ6  # ❌ 错误！应该是 requestTag
Devicetype: 2                                    # ❌ 错误！应该是 deviceType
Deviceuk: 5FQIZE57VAGa7uQBapxU70o3PHzUYI...    # ❌ 错误！应该是 deviceUk
Platform: android                               # ❌ 错误！应该是 platform
```

代码中的设置（`req.Header.Set()` 被规范化了）：
```go
req.Header.Set("DeviceToken", c.deviceToken)   // 被规范化为 Devicetoken
req.Header.Set("requestTag", generateRandomString(32))  // 被规范化为 Requesttag
req.Header.Set("deviceType", "2")               // 被规范化为 Devicetype
req.Header.Set("deviceUk", "...")              // 被规范化为 Deviceuk
req.Header.Set("platform", "android")           // 被规范化为 Platform
```

### ✅ 修改后（正确的 - 保持原始格式）

日志输出中的请求头：
```
DeviceToken: aNbW21QU7cUDAJB4bK22q1rk          # ✓ 正确！
requestTag: 74DFB82D7D50CE91B14806C20A87FE0A  # ✓ 正确！
deviceType: 2                                    # ✓ 正确！
deviceUk: 5FQIZE57VAGa7uQBapxU70o3PHzUYI...   # ✓ 正确！
platform: android                               # ✓ 正确！
```

代码中的设置（直接 map 访问，绕过规范化）：
```go
req.Header["DeviceToken"] = []string{c.deviceToken}  // 保持原始格式
req.Header["requestTag"] = []string{generateRandomString(32)}  // 保持原始格式
req.Header["deviceType"] = []string{"2"}  // 保持原始格式
req.Header["deviceUk"] = []string{"..."}  // 保持原始格式
req.Header["platform"] = []string{"android"}  // 保持原始格式
```

## 关键差异解释

Go 的 `net/http` 包中，`Header.Set()` 方法会自动进行 HTTP Header 规范化：

| Header Key | 用 Set() 设置后 | 直接 map 设置后 |
|-----------|---------------|--------------|
| `DeviceToken` | `Devicetoken` | `DeviceToken` ✓ |
| `requestTag` | `Requesttag` | `requestTag` ✓ |
| `deviceType` | `Devicetype` | `deviceType` ✓ |
| `deviceUk` | `Deviceuk` | `deviceUk` ✓ |
| `platform` | `Platform` | `platform` ✓ |
| `accept-encoding` | `Accept-Encoding` | `accept-encoding` ✓ |

## 为什么重要？

服务器（悠悠有品 API）可能：

1. **严格验证 Header 格式** - 某些反爬虫系统会检查 Header 的精确格式
2. **客户端识别** - 通过 Header 大小写格式来识别是否是真正的 Android 客户端
3. **特征提取** - 将不匹配的 Header 作为爬虫识别的特征

## 测试验证

修改后运行程序，查看日志中的 `[请求头]:` 部分：

```bash
cd /Users/user/Downloads/csgoAuto/cmd/price-monitor
go run main.go -use-proxy=false
```

应该看到类似这样的输出：
```
[请求头]:
  DeviceToken: aNbW21QU7cUDAJB4bK22q1rk        ✓
  DeviceId: aNbW21QU7cUDAJB4bK22q1rk          ✓
  requestTag: ABC123DEF456...                  ✓
  deviceType: 2                                 ✓
  platform: android                            ✓
  deviceUk: 5FQIZE57VA...                     ✓
```

