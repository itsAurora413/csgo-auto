# RSA 密钥生成工具快速启动指南

## 📋 概述

这个工具用于生成 RSA 公私钥对，用于与悠悠有品 OpenAPI 进行通信。

## 🚀 快速使用

### 步骤 1：生成密钥对

```bash
cd cmd/rsa-keygen
go run main.go generate -output ./rsa_keys
```

输出：
- `rsa_keys/public_key_base64.txt` - 公钥
- `rsa_keys/private_key_base64.txt` - 私钥

### 步骤 2：测试签名

```bash
go run main.go test -privatekey ./rsa_keys/private_key_base64.txt -appkey 123456
```

### 步骤 3：在代码中使用

```go
import youpin "csgo-trader/internal/services/youpin"

// 创建签名器
signer, err := youpin.NewRSASigner(privateKeyBase64, appKey)

// 准备参数
params := map[string]interface{}{
    "timestamp":    "2023-12-05 16:15:00",
    "idempotentId": "unique_id",
    // ... 其他参数
}

// 添加签名
err = signer.AddSignatureToParams(params, timestamp)

// 现在 params 包含 sign、appKey、timestamp 字段，可以发送到 API
```

## 📁 文件位置

| 文件 | 功能 |
|------|------|
| `internal/services/youpin/rsa_sign.go` | RSA 签名核心库 |
| `cmd/rsa-keygen/main.go` | 密钥生成示例程序 |

## 🔑 API 方法

### GenerateKeyPair()
生成新的 RSA 密钥对

```go
publicKey, privateKey, err := youpin.GenerateKeyPair()
```

### NewRSASigner()
创建签名器实例

```go
signer, err := youpin.NewRSASigner(privateKeyBase64, appKey)
```

### SignParams()
对参数进行签名

```go
signature, err := signer.SignParams(params)
```

### AddSignatureToParams()
为参数添加签名和公共字段

```go
err := signer.AddSignatureToParams(params, timestamp)
```

## 🔐 安全提示

- ⚠️ 私钥不要提交到 Git
- ⚠️ 私钥泄露要立即重新生成
- ✅ 使用 600 权限保存私钥文件
- ✅ 将私钥存储在环境变量或配置文件

## 📚 更多信息

详见 `RSA_UTILS_README.md`

## 🔗 相关链接

- 官方文档: `/Users/user/Downloads/yyyp-openapi/开放平台准备/RSA公私钥生成、签名.md`
- API 地址: https://gw-openapi.youpin898.com/
