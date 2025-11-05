# 🔧 HTTP Header 大小写修复 - 快速指南

## 问题简述

**为什么你的请求失败而抓包的请求成功？**

❌ **你的代码发送**：`Devicetoken`（规范化后的格式）
✅ **抓包实际发送**：`DeviceToken`（原始格式）

服务器只接受原始格式，所以拒绝了你的请求。

---

## 技术原因

Go 的 `http.Header.Set()` 方法会自动规范化 Header Key：

```go
// Go 自动规范化示例
req.Header.Set("DeviceToken", "value")     // 变成 Devicetoken
req.Header.Set("requestTag", "value")      // 变成 Requesttag
req.Header.Set("accept-encoding", "gzip")  // 变成 Accept-Encoding
```

---

## 解决方案

使用直接 map 访问来保持原始格式：

```go
// 错误方式 ❌
req.Header.Set("DeviceToken", value)

// 正确方式 ✅
req.Header["DeviceToken"] = []string{value}
```

---

## 修改内容

### 已修改的 4 个函数

| 函数名 | 位置 | 修改数量 |
|-------|------|--------|
| `makeRequestWithGzip` | 行 1369-1407 | 18 个 |
| `SendSMSCode` | 行 1604-1617 | 14 个 |
| `LoginWithPhone` | 行 1694-1707 | 14 个 |
| `makeOpenAPIRequest` | 行 2216-2217 | 2 个 |

**总计**：48 处修改

---

## 验证修改

### 编译检查
```bash
cd /Users/user/Downloads/csgoAuto
go build -o /tmp/test ./cmd/price-monitor/main.go
echo $?  # 0 表示成功
```

### 运行测试
```bash
cd /Users/user/Downloads/csgoAuto/cmd/price-monitor
go run main.go
```

### 查看日志
在输出的 `[请求头]:` 部分，应该看到：
- ✅ `DeviceToken: ...` （不是 `Devicetoken`）
- ✅ `requestTag: ...` （不是 `Requesttag`）
- ✅ `deviceType: ...` （不是 `Devicetype`）
- ✅ `platform: ...` （不是 `Platform`）

---

## 修改前后对比

### 修改前的日志（❌ 错误）
```
[请求头]:
  Devicetoken: aNbW21QU7cUDAJB4bK22q1rk
  Deviceid: aNbW21QU7cUDAJB4bK22q1rk
  Requesttag: A6GEMCX3UDOE9KW3QNSS0FTI48CYFUQ6
  Devicetype: 2
  Platform: android
```

### 修改后的日志（✅ 正确）
```
[请求头]:
  DeviceToken: aNbW21QU7cUDAJB4bK22q1rk
  DeviceId: aNbW21QU7cUDAJB4bK22q1rk
  requestTag: A6GEMCX3UDOE9KW3QNSS0FTI48CYFUQ6
  deviceType: 2
  platform: android
```

---

## 为什么这很重要？

悠悠有品 API 使用 Header 格式进行：
1. **客户端识别** - 识别是否是真实的 Android 客户端
2. **反爬虫检测** - 检测爬虫特征
3. **请求验证** - 严格验证 Header 格式

如果 Header 格式不对，服务器会：
- ❌ 返回 `85100 - 系统繁忙,请稍后再试`
- ❌ 拒绝请求
- ❌ 将你的 IP 加入黑名单

---

## 备份和恢复

原始文件已备份：
```bash
ls -la /Users/user/Downloads/csgoAuto/internal/services/youpin/
# 查看 client.go.backup
```

如需恢复：
```bash
cp client.go.backup client.go
```

---

## 相关文档

- `REQUEST_HEADERS_ANALYSIS.md` - 详细的差异分析
- `BEFORE_AFTER_COMPARISON.md` - 修改前后对比
- `MODIFICATION_SUMMARY.txt` - 完整修改总结

