# 前端技术指标集成指南

## 📱 前端适配完成情况

✅ **完成** - `GoodKlineChart.tsx` 组件已完全适配技术指标功能

## 🎯 新增功能

### 1. 指标选择器 (Indicator Selector)
用户可以灵活选择想要显示的技术指标：
- **分组展示**: 指标按类型分为7个分组
- **全选功能**: 点击分组复选框可一次性选中/取消该分组所有指标
- **单个选择**: 也支持单个指标的选择

### 2. 指标值显示面板 (Indicator Values Panel)
实时显示选中指标的最新值：
- **价格信息**: 显示当前价格、最高价、最低价
- **指标数值**: 根据不同指标格式化显示精度

### 3. 灵活的API调用
根据用户选择的指标，自动构建API查询参数：
```typescript
// 示例：选择 ma5, ma20, rsi
GET /api/v1/csqaq/good/kline?id=4&interval=1d&indicators=ma5,ma20,rsi14
```

## 📊 支持的指标分类

```
【移动平均线】
├─ MA5    - 5周期简单移动平均
├─ MA10   - 10周期简单移动平均
├─ MA20   - 20周期简单移动平均
├─ MA60   - 60周期简单移动平均
└─ MA120  - 120周期简单移动平均

【指数平均线】
├─ EMA12  - 12周期指数移动平均
└─ EMA26  - 26周期指数移动平均

【MACD】
├─ MACD      - MACD主线
├─ Signal    - 信号线
└─ Histogram - 直方图

【RSI】
└─ RSI14 - 相对强弱指数

【布林带】
├─ BB上   - 布林带上轨
├─ BB中   - 布林带中线
└─ BB下   - 布林带下轨

【KDJ】
├─ K值 - KDJ的K值
├─ D值 - KDJ的D值
└─ J值 - KDJ的J值

【ATR】
└─ ATR14 - 14周期平均真实波幅
```

## 🔧 技术实现细节

### 组件状态管理

```typescript
// 选中的指标列表
const [selectedIndicators, setSelectedIndicators] = useState<string[]>(
  ['ma5', 'ma20', 'rsi14']  // 默认指标
);

// 最后一根K线的完整数据（包含指标）
const [lastKline, setLastKline] = useState<KPoint | null>(null);

// K线数据列表
const [klineData, setKlineData] = useState<KPoint[]>([]);
```

### 数据流

```
用户选择指标
    ↓
selectedIndicators 状态更新
    ↓
触发 useEffect 重新加载数据
    ↓
构建 indicators 查询参数
    ↓
调用 API 获取指标数据
    ↓
保存到 lastKline 和 klineData
    ↓
UI 渲染指标值
```

### 指标值格式化

```typescript
// 不同指标的精度不同
- RSI/KDJ: 2位小数
- MACD: 4位小数  
- MA/BB/ATR: 2位小数
- 无数据时: 显示 '-'
```

## 📝 使用示例

### 基础使用
1. 打开商品详情页面
2. 在"技术指标"部分选择需要的指标
3. 在"最新指标值"面板查看当前指标值

### 快速操作
- **全选MA**: 点击"移动平均线"分组复选框
- **清空指标**: 点击分组复选框取消选中
- **切换时间间隔**: 使用K线图上方的时间间隔按钮

## 🎨 UI组件说明

### 指标选择器卡片
```
┌─ 技术指标 ────────────────────┐
│  ☑ 移动平均线                  │
│    ☐ MA5  ☐ MA10  ☐ MA20     │
│    ☐ MA60 ☐ MA120            │
│  ☑ 指数平均线                  │
│    ☐ EMA12 ☐ EMA26           │
│  ... (其他分组)                │
└──────────────────────────────┘
```

### 指标值展示卡片
```
┌─ 最新指标值 ──────────────────┐
│  价格: 100.50  最高: 102.30   │
│  最低: 99.80                   │
│                                │
│  MA5: 100.85   MA20: 99.95    │
│  RSI14: 65.42                 │
│  ... (其他选中的指标)           │
└──────────────────────────────┘
```

## 🔄 默认指标配置

组件默认选中以下指标：
- `ma5` - 5周期移动平均线
- `ma20` - 20周期移动平均线
- `rsi14` - 相对强弱指数

可通过修改以下代码自定义默认指标：
```typescript
const [selectedIndicators, setSelectedIndicators] = useState<string[]>(
  ['ma5', 'ma20', 'rsi14']  // 在此修改
);
```

## 🚀 性能优化建议

### 1. 缓存策略
```typescript
// 可在本地缓存选择的指标偏好
const savePreference = (indicators: string[]) => {
  localStorage.setItem('preferredIndicators', JSON.stringify(indicators));
};
```

### 2. 防止过度请求
```typescript
// useEffect 已包含依赖数组控制，避免重复请求
}, [goodId, interval, selectedIndicators]);
```

### 3. 响应数据筛选
```typescript
// 通过 indicators 参数，只获取需要的指标
const indicatorsParam = selectedIndicators.join(',');
```

## 🐛 常见问题

### Q: 为什么某个指标显示 '-'？
**A**: 数据不足导致指标无法计算。建议：
- 增加时间范围（切换到1天粒度）
- 查询更长时间段的数据

### Q: 如何添加新的指标？
**A**: 修改以下部分：
1. `INDICATOR_GROUPS` - 添加分组
2. `INDICATOR_LABELS` - 添加标签
3. `KPoint` 接口 - 添加字段类型

### Q: 指标值精度不准确怎么办？
**A**: 检查 `formatIndicatorValue` 函数中的精度设置

## 📚 相关文件

- **UI组件**: `web/src/components/GoodKlineChart.tsx`
- **API服务**: `web/src/services/marketApiService.ts`
- **API端点**: `internal/api/api.go` (GetGoodKline 方法)
- **指标计算**: `internal/services/technical_indicators.go`

## ✅ 集成检查清单

- [x] 指标选择器 UI 实现
- [x] 指标值展示面板
- [x] 灵活的 API 调用
- [x] 指标值格式化
- [x] 默认指标配置
- [x] 分组管理功能
- [x] 响应式布局
- [x] TypeScript 类型定义

## 🎓 学习路径

### 初级开发者
1. 查看 `GoodKlineChart.tsx` 的基本结构
2. 理解 `selectedIndicators` 状态管理
3. 尝试修改默认指标

### 中级开发者
1. 学习指标选择器的实现原理
2. 理解 API 查询参数构建
3. 自定义指标展示格式

### 高级开发者
1. 在 Chart 上绘制指标线
2. 实现指标交互功能
3. 集成更多高级指标

## 🔮 未来优化方向

### 短期
- [ ] 保存用户指标偏好到本地存储
- [ ] 添加指标说明提示信息
- [ ] 支持自定义 MA 周期

### 中期
- [ ] 在图表上绘制指标线
- [ ] 指标告警功能
- [ ] 指标组合策略

### 长期
- [ ] 实时 WebSocket 推送
- [ ] 指标历史对比
- [ ] AI 推荐指标组合

---

**最后更新**: 2025-10-21  
**版本**: 1.0  
**状态**: ✅ 完成
