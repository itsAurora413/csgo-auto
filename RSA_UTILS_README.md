# RSA 公私钥生成与签名工具使用指南

本文档介绍如何使用项目中的 RSA 工具类来生成公私钥、进行签名和验证。

## 文件位置

- **RSA 签名工具**: `internal/services/youpin/rsa_sign.go`
- **示例程序**: `cmd/rsa-keygen/main.go`
- **官方参考文档**: `/Users/user/Downloads/yyyp-openapi/开放平台准备/RSA公私钥生成、签名.md`

## 核心组件

### 1. GenerateKeyPair() 函数

生成 RSA 公私钥对（Base64 格式）。

```go
publicKey, privateKey, err := youpin.GenerateKeyPair()
if err != nil {
    log.Fatalf("生成密钥失败: %v", err)
}

log.Println("公钥:", publicKey)
log.Println("私钥:", privateKey)
```

### 2. RSASigner 结构体

用于进行 RSA 签名的主要类，负责参数排序、拼接和签名。

```go
// 创建签名器
signer, err := youpin.NewRSASigner(privateKeyBase64, appKey)
if err != nil {
    log.Fatalf("创建签名器失败: %v", err)
}

// 对参数进行签名
params := map[string]interface{}{
    "timestamp":    "2023-12-05 16:15:00",
    "idempotentId": "202212050001",
    // ... 其他参数
}

signature, err := signer.SignParams(params)
if err != nil {
    log.Fatalf("签名失败: %v", err)
}

log.Println("签名:", signature)
```

## 快速开始

### 1. 生成 RSA 密钥对

使用示例程序生成密钥：

```bash
cd cmd/rsa-keygen

# 生成密钥对
go run main.go generate -output ./rsa_keys

# 输出文件：
# - rsa_keys/public_key_base64.txt   (Base64 格式公钥)
# - rsa_keys/private_key_base64.txt  (Base64 格式私钥)
```

### 2. 测试签名功能

使用示例程序测试签名：

```bash
# 测试签名
go run main.go test -privatekey ./rsa_keys/private_key_base64.txt -appkey 123456
```

### 3. 在代码中使用

完整的签名示例：

```go
package main

import (
    "log"
    "time"
    youpin "csgo-trader/internal/services/youpin"
)

func main() {
    // Step 1: 生成密钥对
    publicKey, privateKey, err := youpin.GenerateKeyPair()
    if err != nil {
        log.Fatalf("生成密钥失败: %v", err)
    }
    log.Println("私钥:", privateKey)
    log.Println("公钥:", publicKey)

    // Step 2: 创建签名器
    appKey := "your_app_key"
    signer, err := youpin.NewRSASigner(privateKey, appKey)
    if err != nil {
        log.Fatalf("创建签名器失败: %v", err)
    }

    // Step 3: 准备参数
    params := map[string]interface{}{
        "timestamp":    time.Now().Format("2006-01-02 15:04:05"),
        "idempotentId": "unique_id_12345",
        "purchasingInfoList": []map[string]interface{}{
            {
                "commodityId":    28347880,
                "commodityPrice": 0.12,
                "tradeLinks":     "https://steamcommunity.com/tradeoffer/new/?partner=123&token=ABC",
            },
        },
    }

    // Step 4: 添加签名
    err = signer.AddSignatureToParams(params, time.Now().Format("2006-01-02 15:04:05"))
    if err != nil {
        log.Fatalf("添加签名失败: %v", err)
    }

    // 现在 params 包含 appKey、timestamp 和 sign 字段
    // 可以直接发送到 API
    log.Println("参数已准备完毕，可以发送到 API")
}
```

## RSASigner 主要方法

| 方法 | 描述 | 返回值 |
|------|------|--------|
| `NewRSASigner(privateKey, appKey)` | 创建签名器实例 | `(*RSASigner, error)` |
| `SignParams(params)` | 对参数进行签名 | `(signature string, error)` |
| `AddSignatureToParams(params, timestamp)` | 为参数添加签名及公共字段 | `error` |
| `GetAppKey()` | 获取 AppKey | `string` |

## 签名流程说明

### 签名算法：SHA256withRSA

1. **参数准备**: 准备所有 API 参数（不包含 sign 字段）
2. **参数排序**: 按参数名的 ASCII 码顺序排序
3. **字符串拼接**: `key1 + JSON(value1) + key2 + JSON(value2) + ...`
4. **计算哈希**: 使用 SHA256 计算拼接字符串的哈希值
5. **RSA 签名**: 使用私钥进行 RSA 签名（PKCS#1 v1.5）
6. **Base64 编码**: 将签名结果进行 Base64 编码

### 例子

请求参数：
```json
{
  "timestamp":    "2023-12-05 16:15:00",
  "appKey":       "123456",
  "idempotentId": "202212050001",
  "purchasingInfoList": [{"commodityId": 28347880, "commodityPrice": 0.12, "tradeLinks": "..."}]
}
```

排序后的参数（按 ASCII 码）：
- appKey
- idempotentId
- purchasingInfoList
- timestamp

拼接字符串：
```
appKey"123456"idempotentId"202212050001"purchasingInfoList[{"commodityId":28347880,...}]timestamp"2023-12-05 16:15:00"
```

## 密钥规格

- **密钥大小**: 2048 位（RSA-2048）
- **公钥格式**: PKIX (X.509, RFC 5280)
- **私钥格式**: PKCS#8
- **签名算法**: SHA256withRSA
- **编码方式**: Base64

## API 集成示例

将签名集成到完整的 API 调用中：

```go
package main

import (
    "bytes"
    "encoding/json"
    "io"
    "log"
    "net/http"
    "time"
    youpin "csgo-trader/internal/services/youpin"
)

func callYouyouAPI(apiURL, appKey, privateKey string, businessParams map[string]interface{}) error {
    // 创建签名器
    signer, err := youpin.NewRSASigner(privateKey, appKey)
    if err != nil {
        return err
    }

    // 准备参数
    params := businessParams
    timestamp := time.Now().Format("2006-01-02 15:04:05")

    // 添加签名
    err = signer.AddSignatureToParams(params, timestamp)
    if err != nil {
        return err
    }

    // 序列化为 JSON
    body, err := json.Marshal(params)
    if err != nil {
        return err
    }

    // 发送请求
    resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // 读取响应
    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }

    log.Printf("响应状态码: %d\n", resp.StatusCode)
    log.Printf("响应内容: %s\n", string(respBody))

    return nil
}

func main() {
    // 您的配置
    const appKey = "your_app_key"
    const privateKey = "your_private_key_base64"
    const apiURL = "https://gw-openapi.youpin898.com/open/v1/api/goodsQuery"

    // 业务参数
    businessParams := map[string]interface{}{
        "templateId": "your_template_id",
    }

    // 调用 API
    if err := callYouyouAPI(apiURL, appKey, privateKey, businessParams); err != nil {
        log.Fatalf("API 调用失败: %v", err)
    }
}
```

## 悠悠有品 API 端点

- **商品查询**: `https://gw-openapi.youpin898.com/open/v1/api/goodsQuery`
- **模板查询**: `https://gw-openapi.youpin898.com/open/v1/api/templateQuery`
- **购买**: `https://gw-openapi.youpin898.com/open/v1/api/purchase`

## 安全建议

⚠️ **重要**：

1. **私钥保管**
   - 私钥不要提交到版本控制系统
   - 存储在安全的配置文件或环境变量中
   - 使用受限的文件权限（600）保存

2. **防止泄露**
   - 一旦私钥泄露，立即生成新密钥对
   - 更新悠悠有品平台上的公钥

3. **密钥轮换**
   - 定期更换密钥以增强安全性
   - 建议每年或在怀疑泄露时轮换

## 常见问题

**Q: 生成的密钥每次都不同吗？**
A: 是的，每次调用 `GenerateKeyPair()` 都会生成新的密钥对。

**Q: 可以使用其他工具生成的密钥吗？**
A: 可以，只要密钥符合 PKCS#8（私钥）和 PKIX（公钥）格式，并进行 Base64 编码。

**Q: 签名长度是多少？**
A: RSA-2048 签名经过 Base64 编码后通常是 344 字符。

**Q: 如何验证密钥对是否正确？**
A: 用私钥签名后，计算哈希值并用对应的公钥验证，如果验证成功则密钥对正确。

## 参考资源

- 项目源代码: `internal/services/youpin/rsa_sign.go`
- 官方文档: `RSA公私钥生成、签名.md`
- 悠悠有品 OpenAPI: https://gw-openapi.youpin898.com/
