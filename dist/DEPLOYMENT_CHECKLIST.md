# ✅ 部署检查清单

**版本**: v1.4 (2025-10-18)
**发行包**: trading-system-linux-x86_64.tar.gz (17M)

---

## 📦 发行包验证

```bash
# 1. 检查文件完整性
tar -tzf dist/trading-system-linux-x86_64.tar.gz | wc -l
# 应该显示: 13 行 (包括目录)

# 2. 检查关键二进制
tar -tzf dist/trading-system-linux-x86_64.tar.gz | grep -E "analyzer|seller|daemon"
# 应该显示三个可执行文件

# 3. 检查文档
tar -tzf dist/trading-system-linux-x86_64.tar.gz | grep -E "\.md|\.txt|\.sh"
# 应该显示文档和脚本
```

## 🔍 部署前检查

### 第1步：提取和权限
```bash
□ tar -xzf trading-system-linux-x86_64.tar.gz
□ cd trading-system-linux
□ chmod +x analyzer seller daemon run.sh
□ ls -l | grep analyzer/seller/daemon (验证权限为 755)
```

### 第2步：数据库配置
```bash
□ cp .env.example .env
□ 编辑 .env 文件，填入数据库连接信息:
  - 服务器 IP/地址
  - 端口
  - 数据库名
  - 用户名/密码
□ 测试连接: mysql -u <user> -p <password> -h <host> <database>
```

### 第3步：依赖检查
```bash
□ 检查是否安装了 MySQL/MariaDB
□ 检查数据库中的表:
  - arbitrage_opportunities
  - arbitrage_opportunities_history
  - holding_position
  - purchase_plan
□ 检查表结构中是否包含:
  - price_trend (analyzer v1.3+ 添加)
  - risk_level (analyzer v1.3+ 添加)
  - score (用于排序)
```

### 第4步：磁盘和内存
```bash
□ 可用磁盘空间 >= 1GB (用于日志和缓存)
□ 可用内存 >= 512MB
□ 网络连接正常 (能访问 YouPin API)
```

## 🚀 部署步骤

### 启动序列
```bash
# Step 1: 在终端 1 启动 analyzer (前台,便于观察)
./analyzer -budget 100

# Step 2: 在终端 2 启动 seller (后台)
nohup ./seller > seller.log 2>&1 &

# Step 3: 在终端 3 启动 daemon (后台)
nohup ./daemon -interval 5m -days 7 > daemon.log 2>&1 &

# Step 4: 验证所有进程已启动
ps aux | grep -E "analyzer|seller|daemon" | grep -v grep
# 应该显示 3 个进程
```

### 快速验证 (3 分钟)
```bash
□ analyzer 有输出 (正在分析机会)
□ seller.log 显示正常日志
□ daemon.log 有"数据库连接成功"
□ 没有错误信息
```

## 📊 运行监控

### 第1小时
```
□ analyzer 生成 >50 个机会
□ seller 显示上架日志
□ daemon 显示"找到 0 个活跃持仓"(正常,第一次)
□ 无数据库错误
□ 无连接超时错误
```

### 第1天
```
□ analyzer 共生成 >200 个机会 (4 次运行)
□ seller 显示至少 1 笔成交
□ daemon 多次执行,无错误
□ 日志文件大小正常增长
```

### 第1周
```
□ analyzer 共生成 >1000 个机会
□ seller 完成 >5 笔成交
□ daemon 继续监控,无异常
□ 数据库没有表空间问题
```

### 第2周 (daemon 开始有输出)
```
□ daemon 输出 7 天的回测数据
□ 显示向上/向下/稳定趋势统计
□ 显示胜率和利润对比
□ 可能显示改进建议 (如果有问题)
```

## 🔧 常见问题排查

### 问题1: 数据库连接失败
```
错误信息: "数据库连接失败"

排查步骤:
□ 检查 .env 文件的连接信息
□ 测试 mysql 连接: mysql -u <user> -p <pass> -h <host>
□ 确认防火墙允许 3306 端口
□ 确认数据库服务正在运行
□ 检查用户权限是否足够
```

### 问题2: analyzer 无输出
```
错误信息: analyzer 运行但没有生成机会

排查步骤:
□ 检查是否有网络连接错误
□ 检查 YouPin API 配置是否正确
□ 查看 analyzer 的具体错误信息
□ 确认 app_key 和 private_key 有效
□ 检查账户是否被限制
```

### 问题3: daemon 无输出
```
错误信息: daemon 运行但没有输出

原因可能:
□ analyzer 还未生成数据 (新部署)
□ 没有 7 天的历史数据 (需要等待)
□ 数据库中没有 arbitrage_opportunities 记录

等待:
  如果是新部署,daemon 需要等待:
  - 1-2 天: analyzer 生成数据
  - 7+ 天: 有足够的历史数据供回测

快速测试 (可选):
  使用 -days 0 参数查看最近 1 天的数据:
  ./daemon -days 0
```

### 问题4: 高内存使用
```
错误信息: daemon 占用 >500MB 内存

排查步骤:
□ 检查是否有内存泄漏 (停止后内存释放)
□ 减少 -days 参数 (默认 7 天)
□ 减少数据库查询范围
□ 重启 daemon
```

## 📝 日志检查

### analyzer.log 应该显示
```
✅ 【成功】
   • 找到 XXX 个套利机会
   • 保存到数据库: YYY 条
   • 详细日志:
     - 物品名称
     - 趋势 (📈/📉/→)
     - 风险等级
     - 预期利润

❌ 【错误】
   • "数据库连接失败"
   • "API 返回 401"
   • "无法解析 JSON"
```

### seller.log 应该显示
```
✅ 【成功】
   • 上架: XXX 个商品
   • 降价: YYY 次
   • 成交: ZZZ 笔

❌ 【错误】
   • "账户余额不足"
   • "上架失败 429"
   • "网络超时"
```

### daemon.log 应该显示
```
✅ 【成功】
   • 找到 XXX 个历史记录
   • 分析 YYY 天前的数据
   • 趋势分类完成
   • 胜率统计: XXX%
   • 【策略反馈】

❌ 【错误】
   • "暂无 7 天的历史数据"
   • "数据库查询失败"
   • "字段不存在"
```

## 🔄 后续优化

### 第1周 - 观察阶段
```
□ 监控三个模块的运行
□ 收集日志分析问题
□ 等待 daemon 首次回测
□ 记录任何异常
```

### 第2周 - 反馈阶段
```
□ daemon 输出第一份回测报告
□ 查看是否有 【严重问题】 标记
□ 如有问题,记录具体建议
□ 准备改进 analyzer
```

### 第3周 - 优化阶段
```
□ 根据 daemon 建议修改 analyzer
□ 重新编译部署
□ 监控新数据的变化
□ 持续调整参数
```

## ✅ 最终检查

部署前最后确认:

```bash
□ 三个二进制都能执行
  file trading-system-linux/{analyzer,seller,daemon}
  # 应该都显示: ELF 64-bit LSB executable

□ 数据库连接正常
  mysql -u root -p*** -h 127.0.0.1 csgo_trader -e "SHOW TABLES;"
  # 应该显示所有表

□ 磁盘空间充足
  df -h
  # / 分区应该有 >1GB 可用空间

□ 没有过时进程
  ps aux | grep -E "analyzer|seller|daemon"
  # 应该没有旧进程在运行

□ 配置文件正确
  cat .env | grep -E "DB_|YOUPIN_"
  # 应该显示有效的配置值
```

---

**完成本清单后,系统已准备好正式部署！**

如有问题,查看相应模块的详细文档:
- TREND_ANALYSIS_LOGGING.md (analyzer)
- DAEMON_BACKTEST_ENHANCEMENT.md (daemon)
- SELLING_API_README.md (seller)
