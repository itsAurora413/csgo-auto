# 求购监控脚本使用说明

## 功能介绍

求购监控脚本用于定时检查悠悠有品的求购订单，并自动执行以下操作：

1. **检查利润率**：删除利润率不符合要求的求购订单
2. **竞争排名**：当不是第一名且有利可图时，自动加价抢占第一名
3. **优化成本**：当是第一名且与第二名差距过大时，适当降价节省成本

## 核心逻辑

### 1. 利润率检查
- 获取当前市场最低售价
- 计算预期利润：`售价 - 售价×2%手续费 - 求购价`
- 如果利润率低于设定阈值（默认5%），删除求购订单

### 2. 排名竞争（阶梯式加价）
**当前不是第一名时：**
- 检查第一名的价格与售价差值
- 如果第一名利润率太低（<最小利润率的50%），不竞争
- 否则使用阶梯式加价策略超过第一名
- 确保加价后仍有足够利润率

**阶梯式加价规则：**
| 价格区间 | 加价增量 | 示例 |
|---------|---------|------|
| 0~1元 | 0.01元 | 0.85元 → 0.86元 |
| 1~50元 | 0.1元 | 10.5元 → 10.6元 |
| 50~1000元 | 1元 | 100元 → 101元 |
| 1000元以上 | 10元 | 1500元 → 1510元 |

**示例1（低价商品）：**
```
当前排名: 第2名，求购价: ¥0.85
第一名求购价: ¥0.86
加价后: ¥0.87 (增量0.01)
```

**示例2（中价商品）：**
```
当前排名: 第2名，求购价: ¥100
第一名求购价: ¥102
市场最低售价: ¥110
第一名利润率: (110 - 110×0.02 - 102) / 102 = 5.88%
加价后: ¥103 (增量1元)
新利润率: (110 - 110×0.02 - 103) / 103 = 4.66%
```

**示例3（高价商品）：**
```
当前排名: 第2名，求购价: ¥1500
第一名求购价: ¥1520
加价后: ¥1530 (增量10元)
```

### 3. 成本优化
**当前是第一名时：**
- 检查与第二名的价格差距
- 如果差距过大（>最小差距的2倍，默认>4%），适当降价
- 降价到比第二名高一点（默认高1%）
- 确保降价后仍有足够利润率

**示例：**
```
当前排名: 第1名，求购价: ¥105
第二名求购价: ¥100
价格差距: (105 - 100) / 100 = 5%
降价后: ¥100 × 1.01 = ¥101
节省成本: ¥105 - ¥101 = ¥4
```

## 使用方法

### 前置条件
1. 确保已经设置YouPin Token环境变量
2. 确保数据库连接正常
3. 确保相关API已经对接完成

### 快速开始

```bash
# 设置Token
export YOUPIN_TOKEN='your_token_here'

# 演练模式（不实际修改，只查看会执行什么操作）
./scripts/run_purchase_monitor.sh --dry-run --once

# 正式运行一次
./scripts/run_purchase_monitor.sh --once

# 启动循环监控（每5分钟检查一次）
./scripts/run_purchase_monitor.sh
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--token <token>` | YouPin API Token | 从环境变量读取 |
| `--interval <seconds>` | 检查间隔（秒） | 300（5分钟） |
| `--dry-run` | 演练模式，不实际修改 | false |
| `--once` | 只运行一次，不循环 | false |
| `--help` | 显示帮助信息 | - |

### 高级参数

在脚本中可以修改以下参数：

```bash
MIN_PROFIT_RATE=0.05      # 最小利润率 5%
PRICE_INCREMENT=0.01      # 加价幅度 1%
PRICE_DECREMENT=0.005     # 降价幅度 0.5%
MIN_RANK_GAP=0.02         # 第一名与第二名最小差距 2%
```

或直接使用Go程序：

```bash
go run cmd/purchase-monitor/main.go \
    -token="your_token" \
    -interval=300 \
    -min-profit=0.05 \
    -decrement=0.005 \
    -min-rank-gap=0.02 \
    -dry-run
```

## 加价策略详解

### 阶梯式加价
脚本采用阶梯式加价策略，根据价格区间使用不同的增量：

1. **0~1元商品**（如贴纸、低价皮肤）
   - 增量：0.01元
   - 原因：低价商品对小额变化敏感，使用最小增量

2. **1~50元商品**（如普通武器皮肤）
   - 增量：0.1元
   - 原因：主流价位商品，保持合理的价格竞争力

3. **50~1000元商品**（如高级武器皮肤）
   - 增量：1元
   - 原因：高价商品价格变化较大，使用整数增量

4. **1000元以上商品**（如珍稀皮肤、刀具）
   - 增量：10元
   - 原因：顶级商品市场，价格波动较大

### 加价计算方法
```go
// 示例：第一名是102元
// 1. 向上取整到增量的倍数：ceil(102/1) = 102
// 2. 加一个增量：102 + 1 = 103元
```

## 日志说明

日志文件位置：`logs/purchase_monitor.log`

### 日志示例

```
[PURCHASE-MONITOR] 2025-10-12 10:00:00 ================== 求购监控脚本启动 ==================
[PURCHASE-MONITOR] 2025-10-12 10:00:00 YouPin账户: 测试账号
[PURCHASE-MONITOR] 2025-10-12 10:00:00 数据库连接成功
[PURCHASE-MONITOR] 2025-10-12 10:00:00 ========== 开始检查求购订单 ==========
[PURCHASE-MONITOR] 2025-10-12 10:00:01 找到 15 个求购订单，开始检查...
[PURCHASE-MONITOR] 2025-10-12 10:00:02 [1/15] 检查订单: ORDER123 - AK-47 | 红线 (久经沙场)
[PURCHASE-MONITOR] 2025-10-12 10:00:02   排名: 2 | 价格: ¥100.00 | 最低售价: ¥110.00 | 利润率: 7.84%
[PURCHASE-MONITOR] 2025-10-12 10:00:02   决策: increase - 排名第2，加价¥1.00抢第一（新利润率6.79%）
[PURCHASE-MONITOR] 2025-10-12 10:00:02   >> 更新求购价格: ¥100.00 -> ¥103.00
[PURCHASE-MONITOR] 2025-10-12 10:00:03   >> 更新成功
...
[PURCHASE-MONITOR] 2025-10-12 10:05:00 ========== 检查完成 ==========
[PURCHASE-MONITOR] 2025-10-12 10:05:00 总订单数: 15
[PURCHASE-MONITOR] 2025-10-12 10:05:00 保持不变: 10
[PURCHASE-MONITOR] 2025-10-12 10:05:00 删除订单: 2
[PURCHASE-MONITOR] 2025-10-12 10:05:00 加价调整: 2
[PURCHASE-MONITOR] 2025-10-12 10:05:00 降价调整: 1
[PURCHASE-MONITOR] 2025-10-12 10:05:00 处理错误: 0
[PURCHASE-MONITOR] 2025-10-12 10:05:00 耗时: 3m2s
```

## 注意事项

### 安全性
1. **先使用演练模式测试**：`--dry-run`参数可以查看脚本会执行什么操作，而不实际修改
2. **先使用单次运行**：`--once`参数只运行一次，观察结果后再启用循环模式
3. **使用灰度发布**：从小比例开始，逐步扩大范围

### 运行频率
- 建议间隔不要太短（至少3-5分钟），避免频繁调用API
- 可以根据市场活跃度调整间隔
- 夜间可以适当延长间隔

### 参数调整
- `MIN_PROFIT_RATE`: 根据市场情况和个人风险偏好调整（默认5%）
- `PRICE_DECREMENT`: 降价幅度太大可能失去第一名位置（默认0.5%）
- `MIN_RANK_GAP`: 控制第一名和第二名的合理差距（默认2%）

注意：加价策略已内置为阶梯式，无需配置增量参数

### 异常处理
- 脚本会自动记录错误日志
- 单个订单处理失败不影响其他订单
- 建议配合监控告警系统使用

## 配合使用

可以配合其他脚本使用：

```bash
# 1. 先运行套利分析，找到机会
./arbitrage-analyzer-optimized -once -budget 20000

# 2. 手动创建求购订单（或通过Web界面）

# 3. 启动监控脚本，自动维护求购订单
./scripts/run_purchase_monitor.sh
```

## 部署建议

### Systemd服务

创建文件 `/etc/systemd/system/purchase-monitor.service`：

```ini
[Unit]
Description=CSGO Purchase Monitor
After=network.target

[Service]
Type=simple
User=your_user
WorkingDirectory=/path/to/csgoAuto
Environment="YOUPIN_TOKEN=your_token_here"
ExecStart=/path/to/csgoAuto/scripts/run_purchase_monitor.sh
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable purchase-monitor
sudo systemctl start purchase-monitor
sudo systemctl status purchase-monitor
```

### Cron任务

定时运行（每5分钟）：

```bash
# 编辑crontab
crontab -e

# 添加任务
*/5 * * * * cd /path/to/csgoAuto && YOUPIN_TOKEN='your_token' ./scripts/run_purchase_monitor.sh --once >> logs/cron.log 2>&1
```

## 故障排查

### 无法获取求购列表
- 检查Token是否有效
- 检查网络连接
- 查看API返回的错误信息

### 更新失败
- 检查订单状态（可能已被取消）
- 检查余额是否足够
- 检查价格是否在合理范围内

### 程序崩溃
- 查看日志文件中的错误信息
- 检查数据库连接
- 确认API接口是否正常

## API依赖

此脚本依赖以下YouPin API：

1. `SearchPurchaseOrderList` - 获取求购订单列表
2. `GetCommodityList` - 获取在售商品列表
3. `GetTemplatePurchaseOrderList` - 获取求购排行榜
4. `GetPurchaseOrderDetail` - 获取订单详情
5. `GetTemplatePurchaseInfo` - 获取模板信息
6. `UpdatePurchaseOrder` - 更新求购订单
7. `DeletePurchaseOrder` - 删除求购订单

**在运行脚本前，请确保这些API已经正确对接和测试！**

## 更新日志

- 2025-10-12: 初始版本，支持基本的求购监控和调整功能
- 2025-10-12: 实现阶梯式加价策略，根据价格区间自动调整增量
