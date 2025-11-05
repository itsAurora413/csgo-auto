# 请求头对比分析：失败 vs 成功请求

## 🔍 核心差异识别

### ❌ 失败请求（日志中的）与 ✅ 成功请求（抓包数据）的对比

#### **请求头构建问题 - Header Key 大小写不一致**

代码中设置的请求头（`client.go` 行 1375-1386）：
```go
req.Header.Set("DeviceToken", c.deviceToken)      // 首字母大写
req.Header.Set("DeviceId", c.deviceID)            // 首字母大写
req.Header.Set("requestTag", strings.ToUpper(...)) // 首字母小写
req.Header.Set("Gameid", "730")                   // 混合大小写
req.Header.Set("deviceType", "2")                 // 首字母小写
req.Header.Set("platform", "android")             // 全小写
req.Header.Set("currentTheme", "Light")           // camelCase
req.Header.Set("package-type", "uuyp")            // kebab-case
```

**问题所在**：Go 的 `http.Header.Set()` 会自动将 Header Key 转换为 **Title Case** 格式（规范化）。

例如：
- `DeviceToken` → 保持 `Devicetoken` ❌
- `deviceType` → 转为 `Devicetype` ❌
- `requestTag` → 转为 `Requesttag` ❌

### ✅ 成功请求中的格式（抓包数据）

```
DeviceToken: aNbW21QU7cUDAJB4bK22q1rk
DeviceId: aNbW21QU7cUDAJB4bK22q1rk
requestTag: 74DFB82D7D50CE91B14806C20A87FE0A
Gameid: 730
deviceType: 2
platform: android
currentTheme: Light
package-type: uuyp
AppType: 4
```

### ❌ 日志中实际发送的请求头（错误的）

```
Deviceid: aNbW21QU7cUDAJB4bK22q1rk           # 应该是 DeviceId
Requesttag: A6GEMCX3UDOE9KW3QNSS0FTI48CYFUQ6 # 应该是 requestTag
Devicetype: 2                                  # 应该是 deviceType
Deviceuk: 5FQIZE57VAGa7uQBapxU70o3PHzUYIUevEmrT53gRd8hMLiEMafT7TmLexlKfk51I # 应该是 deviceUk
Devicetoken: aNbW21QU7cUDAJB4bK22q1rk       # 应该是 DeviceToken
```

---

## 📊 完整对比表

| 请求头名称 | 代码设置 | HTTP规范后 | 抓包正确值 | 匹配? |
|-----------|--------|----------|---------|------|
| Device Token | `DeviceToken` | `Devicetoken` | `DeviceToken` | ❌ |
| Device ID | `DeviceId` | `Deviceid` | `DeviceId` | ❌ |
| Request Tag | `requestTag` | `Requesttag` | `requestTag` | ❌ |
| Device Type | `deviceType` | `Devicetype` | `deviceType` | ❌ |
| Device UK | `deviceUk` | `Deviceuk` | `deviceUk` | ❌ |
| Platform | `platform` | `Platform` | `platform` | ❌ |
| App Version | `App-Version` | `App-Version` | `App-Version` | ✅ |

---

## 🔧 解决方案

### 问题原因

Go 的 `net/http` 包会自动规范化 HTTP Header Key 为 **Canonical Form**：
- 将首字母大写
- 每个连字符后的字母大写
- 其他字母转为小写

这导致代码设置的 Key 名称与服务器期望的格式不匹配。

### 修复方法

需要修改 `client.go` 中的 `makeRequestWithGzip` 函数（约 1370-1407 行）：

**错误方式**（现在的代码）：
```go
req.Header.Set("DeviceToken", c.deviceToken)  // 会变成 Devicetoken
req.Header.Set("requestTag", ...)             // 会变成 Requesttag
```

**正确方式**：
```go
// 方案 A：使用正确的规范化形式
req.Header.Set("Devicetoken", c.deviceToken)    
req.Header.Set("Requesttag", strings.ToUpper(...))
req.Header.Set("Devicetype", "2")
req.Header.Set("Deviceuk", "...")

// 方案 B：直接访问底层 map 设置（绕过规范化）
req.Header["DeviceToken"] = []string{c.deviceToken}
req.Header["requestTag"] = []string{strings.ToUpper(...)}
```

### 推荐修复

使用直接 map 访问方式，以保持原始的大小写格式：

```go
// 替换所有 req.Header.Set 调用，使用直接 map 访问
headers := map[string]string{
    "User-Agent":        "okhttp/3.14.9",
    "Connection":        "Keep-Alive",
    "Accept-Encoding":   "gzip",
    "tracestate":        "bnro=android/10_android/8.20.0_okhttp/3.14.9",
    "traceparent":       fmt.Sprintf("00-%s-%s-01", generateRandomString(32), generateRandomString(16)),
    "DeviceToken":       c.deviceToken,
    "DeviceId":          c.deviceID,
    "requestTag":        strings.ToUpper(generateRandomString(32)),
    "Gameid":            "730",
    "deviceType":        "2",
    "platform":          "android",
    "currentTheme":      "Light",
    "package-type":      "uuyp",
    "App-Version":       "5.37.1",
    "uk":                "5FQFWiQh8VvtSm0krHaYs52HWGSqA0v4UVcWASmLbSD68mdWzxo3oSoRtbSgwY91L",
    "deviceUk":          "5FQIZE57VAGa7uQBapxU70o3PHzUYIUevEmrT53gRd8hMLiEMafT7TmLexlKfk51I",
    "AppType":           "4",
    "Authorization":     "Bearer " + c.token,
    "Content-Type":      "application/json",
}

for key, value := range headers {
    req.Header[key] = []string{value}
}
```

---

## 🎯 为什么抓包的请求能成功？

因为移动设备（安卓 okhttp）直接发送的就是 **正确的大小写格式**，服务器已经被训练接受这种格式。

而 Go 代码的规范化导致发送了错误的 Header 格式，服务器可能：
1. 要求特定的大小写格式用于识别客户端
2. 通过 Header 格式进行客户端验证
3. 使用 Header 格式进行反爬虫检测

---

## 📝 Action Items

1. ✅ 定位问题：HTTP Header Key 大小写不匹配
2. ⏳ 修复代码：更新 `client.go` 的请求头设置方式
3. ⏳ 测试验证：确认修复后请求能够成功

