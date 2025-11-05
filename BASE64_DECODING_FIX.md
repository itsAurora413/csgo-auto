# Base64 私钥解码错误修复总结

## 问题描述

错误信息：
```
2025/10/29 10:04:16 Failed to create YouPin OpenAPI client:创建OpenAPI客户端失败: 创建RSA签名器失败: 解码私钥失败: illegal base64 data at input byte 1624
```

## 根本原因

`csqaq-sampler/internal/services/youpin/openapi.go` 中的 RSA 私钥常量被截断了：

**损坏的密钥**（1625 字节，末尾被破坏）：
```
const YoupinOpenAPIPrivateKey = "MIIEvQIBADANBgkq...xs=%"  // ❌ 以 "xs=%" 结尾，这不是有效的 Base64
```

最后几个字符是 `xs=%`，这导致了非法的 Base64 数据在第 1624 字节处。

**正确的密钥**（1624 字节）：
```
const YoupinOpenAPIPrivateKey = "MIIEvgIBADANBgkq...qWiYa+T"  // ✅ 以有效的 Base64 字符结尾
```

## 修复方案

### 1. 更新私钥常量

**文件：** `csqaq-sampler/internal/services/youpin/openapi.go` 第 19 行

将被截断的私钥替换为完整正确的私钥。

### 2. 改进 RSA 签名器的错误处理

**文件：** 
- `csqaq-sampler/internal/services/youpin/rsa_sign.go`
- `internal/services/youpin/rsa_sign.go`

**改进内容：**

a) **预处理 Base64 字符串**：移除所有空白字符（换行符、制表符、空格）
```go
privateKeyBase64 = strings.TrimSpace(privateKeyBase64)
privateKeyBase64 = strings.Map(func(r rune) rune {
    if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
        return -1
    }
    return r
}, privateKeyBase64)
```

b) **改进错误信息**：提供更详细的诊断信息
```go
return nil, fmt.Errorf("解码私钥失败: %w (私钥长度: %d字节, 错误: %v)", err, len(privateKeyBase64), err)
```

## 验证测试

使用测试脚本验证两个密钥的区别：

```
测试：损坏的密钥（以xs=%结尾）
密钥长度：1625
原始解码失败：illegal base64 data at input byte 1624

测试：正确的密钥
密钥长度：1624
原始解码成功
```

## 修改的文件

1. ✅ `csqaq-sampler/internal/services/youpin/openapi.go` - 更新私钥常量
2. ✅ `csqaq-sampler/internal/services/youpin/rsa_sign.go` - 改进错误处理
3. ✅ `internal/services/youpin/rsa_sign.go` - 改进错误处理

## 预期结果

修复后，OpenAPI 客户端应能够成功初始化，不再出现 Base64 解码错误。

```
YouPin OpenAPI客户端初始化成功 (OpenAPI认证)
```

## 额外建议

如果将来需要在环境变量中配置私钥，建议：

1. **添加验证**：在加载私钥时进行验证
2. **支持多种格式**：支持带/不带换行符的 Base64 编码
3. **日志记录**：在生产环境中记录密钥加载的详细信息
4. **备份管理**：定期备份和轮换 RSA 密钥对
