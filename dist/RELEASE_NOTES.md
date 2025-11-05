# 交易系统 Linux 发行版 - 发行说明

**版本**: 1.1 (带趋势分析修复)
**发布日期**: 2025-10-18 22:54 UTC
**平台**: Linux x86_64
**编译环境**: Go 1.21+

---

## 🔥 最新更新 - 关键修复

### 热修复 2025-10-18：趋势分析修复 ⭐

**问题**: 格洛克 18 型（StatTrak™）| 粉碎者因为趋势分析错误被错误推荐

**原因**:
1. ❌ 用售价而不是买价分析趋势
2. ❌ 线性回归归一化假设不适应不同价格水平
3. ❌ 无法检测短期陡峭价格下跌

**修复**:
1. ✅ 改用买价进行四因子趋势分析
2. ✅ 动态百分比归一化，自适应所有价格水平
3. ✅ 添加最近 6 小时跌幅 >10% 的硬性过滤

**详见**: `HOTFIX_2025_10_18.md` 和 `TREND_FIX_SUMMARY.md`

---

## 📦 发行包内容

### 压缩包: `trading-system-linux-x86_64.tar.gz` (17M)

解压后包含:

```
trading-system-linux/
├── analyzer              (11M) - 分析脚本
├── seller                (8M)  - 出售脚本
├── daemon                (11M) - 守护进程
├── run.sh                (脚本) - 帮助脚本
├── README.md             - Linux 版本说明
├── START_HERE.md         - 快速入门
├── COMMANDS.txt          - 命令参考
├── NEW_WORKFLOW.md       - 详细流程
├── REFACTOR_SUMMARY.md   - 技术细节
└── .env.example          - 配置模板
```

---

## 🚀 快速安装

### 1. 解压
```bash
tar -xzf trading-system-linux-x86_64.tar.gz
cd trading-system-linux
```

### 2. 验证
```bash
./run.sh
# 或手动检查
ls -la analyzer seller daemon
file analyzer  # 验证是 Linux x86_64 二进制
```

### 3. 设置权限 (如需要)
```bash
chmod +x analyzer seller daemon
```

---

## 📋 使用示例

### 例1: 分析机会 (50元预算)
```bash
./analyzer -budget 50
```

### 例2: 出售物品 (需要私钥)
```bash
export YOUPIN_PRIVATE_KEY="$(cat /path/to/private_key.pem)"
./seller -target "P250 | 污染物" -price 23.00
```

### 例3: 启动后台守护进程
```bash
./daemon -interval 5m
```

### 例4: 后台持续运行 (推荐)
```bash
nohup ./daemon > daemon.log 2>&1 &
```

---

## ⚙️ 系统要求

- **操作系统**: Linux (x86_64 架构)
- **glibc 版本**: 2.17 或更高 (RHEL 7+, Debian 8+, Ubuntu 14.04+)
- **内存**: 最少 256MB
- **网络**: 需要互联网连接 (连接远程数据库)
- **磁盘**: 最少 50MB 空闲空间

### 验证 glibc 版本
```bash
ldd --version
# 或
libc --version
```

---

## 🔧 常见问题

### Q: 权限不足
```bash
chmod +x analyzer seller daemon
```

### Q: 找不到库文件
通常是 glibc 版本过旧。升级系统或使用容器。

### Q: 如何后台运行?
```bash
nohup ./daemon > daemon.log 2>&1 &
tail -f daemon.log  # 查看日志
pkill -f daemon     # 停止进程
```

### Q: 需要修改 STEAM_ID?
需要重新编译源代码:
```bash
GOOS=linux GOARCH=amd64 go build -o analyzer cmd/analyzer/main.go
```

---

## 📊 文件校验

| 文件 | 大小 | 说明 |
|------|------|------|
| analyzer | 11M | 分析脚本 |
| seller | 8.4M | 出售脚本 |
| daemon | 11M | 守护进程 |

总大小: ~30.4M (解压后)
压缩包: ~17M

---

## 🔐 安全性

### 私钥管理

**不要**:
- ❌ 不要在命令行中暴露私钥
- ❌ 不要提交私钥到 Git
- ❌ 不要在环境变量中硬编码私钥

**要**:
- ✅ 从文件加载私钥
- ✅ 使用环境变量
- ✅ 定期更换私钥
- ✅ 权限设置: `chmod 600 private_key.pem`

### 推荐设置
```bash
# 将私钥保存到安全位置
cp private_key.pem ~/.youpin_private_key
chmod 600 ~/.youpin_private_key

# 使用时
export YOUPIN_PRIVATE_KEY="$(cat ~/.youpin_private_key)"
./seller -target "..." -price ...
```

---

## 📈 性能指标

| 操作 | 耗时 | 内存 |
|------|------|------|
| analyzer (50元) | ~2-3秒 | ~50MB |
| seller (单件) | ~1秒 | ~30MB |
| daemon (循环) | 持续 | ~20MB |

---

## 🐛 故障排除

### 无法执行二进制文件
```bash
# 检查架构
file analyzer
# 应该显示: ELF 64-bit LSB executable, x86-64

# 检查权限
chmod +x analyzer
```

### 数据库连接失败
```bash
# 检查网络连接
ping 23.254.215.66

# 检查端口
telnet 23.254.215.66 3306
```

### daemon 崩溃
```bash
# 查看日志
tail -f daemon.log

# 重新启动
./daemon -interval 5m
```

---

## 📝 配置说明

### 硬编码配置 (在代码中)
```
STEAM_ID = 76561199078507841
YOUPIN_APP_KEY = 12919014
```

### 环境变量 (仅 seller 需要)
```bash
export YOUPIN_PRIVATE_KEY="<your-private-key>"
```

### 命令行参数

**analyzer**:
```bash
-budget N    预算(元) [默认: 50]
```

**seller**:
```bash
-target "名称"   物品名称 (必需)
-price N        售价(元) [默认: 50]
-qty N          数量 [默认: 1]
```

**daemon**:
```bash
-interval D      检查间隔 [默认: 5m]
-backtest bool   是否回测 [默认: true]
-days N          回测天数 [默认: 7]
```

---

## 🚀 部署建议

### 单机部署
```bash
# 1. 解压
tar -xzf trading-system-linux-x86_64.tar.gz
cd trading-system-linux

# 2. 测试
./analyzer -budget 10

# 3. 生产运行
nohup ./daemon > daemon.log 2>&1 &
```

### Docker 部署 (可选)
```dockerfile
FROM ubuntu:20.04
WORKDIR /app
COPY trading-system-linux/ .
RUN chmod +x analyzer seller daemon
ENTRYPOINT ["./daemon"]
```

### 定时任务 (crontab)
```bash
# 每天早上8点运行分析
0 8 * * * /path/to/trading-system-linux/analyzer -budget 100 >> /tmp/analyzer.log 2>&1
```

---

## 📖 文档

每个文档的推荐阅读顺序:

1. **README.md** (当前目录) - 快速说明
2. **START_HERE.md** - 30秒快速开始
3. **COMMANDS.txt** - 命令参考
4. **NEW_WORKFLOW.md** - 详细流程
5. **REFACTOR_SUMMARY.md** - 技术细节

---

## 🔄 更新和升级

### 获取新版本
```bash
# 下载新压缩包
wget https://example.com/trading-system-linux-x86_64.tar.gz

# 备份旧版本
mv trading-system-linux trading-system-linux.backup

# 解压新版本
tar -xzf trading-system-linux-x86_64.tar.gz

# 迁移配置 (如需要)
cp trading-system-linux.backup/.env trading-system-linux/
```

---

## 📞 支持

遇到问题?

1. 查看 `COMMANDS.txt` 中的常见问题
2. 查看 `daemon.log` 的错误信息
3. 验证网络连接
4. 检查系统要求

---

## 📜 许可证

本软件按原项目许可证分发。

---

## ✅ 发行清单

- ✅ 三个二进制文件 (analyzer, seller, daemon)
- ✅ 完整文档
- ✅ 运行脚本
- ✅ 配置模板
- ✅ 发行说明

---

**准备好了? 解压后运行 `./run.sh` 开始吧!** 🚀

```bash
tar -xzf trading-system-linux-x86_64.tar.gz
cd trading-system-linux
./run.sh
```
