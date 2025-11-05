# 🚀 从这里开始

**新的三脚本系统已准备好！**

---

## ⚡ 30秒快速开始

```bash
# 1. 分析机会 (50元预算)
./bin/analyzer -budget 50

# 2. 在悠悠有品手动发布求购

# 3. 物品到账后出售
export YOUPIN_PRIVATE_KEY="$(cat private_key.pem)"
./bin/seller -target "P250 | 污染物" -price 23.00

# 4. 启动后台守护进程
./bin/daemon

# 完成! 🎉
```

---

## 📚 文档导航

1. **COMMANDS.txt** ← 命令参考 (必读!)
2. **NEW_WORKFLOW.md** ← 详细流程
3. **REFACTOR_SUMMARY.md** ← 技术细节

---

## 🎯 三脚本说明

### 1. 分析脚本 (./bin/analyzer)
- **何时**: 每天/每周分析一次
- **用途**: 发现机会 + 生成订单
- **参数**: `-budget N`

### 2. 出售脚本 (./bin/seller)
- **何时**: 物品到账后
- **用途**: 上架 + 设置价格
- **参数**: `-target "名称"`, `-price N`, `-qty N`

### 3. 守护进程 (./bin/daemon)
- **何时**: 启动后持续运行
- **用途**: 监控 + 止损/止盈 + 调整
- **参数**: `-interval D`, `-backtest bool`

---

## 🔑 关键配置

**硬编码在脚本中**:
- STEAM_ID = 76561199078507841
- YOUPIN_APP_KEY = 12919014

**需要手动设置** (仅seller脚本):
```bash
export YOUPIN_PRIVATE_KEY="$(cat private_key.pem)"
```

---

## 💡 工作流

```
DAY 1 早上
├─ 运行: ./bin/analyzer -budget 50
└─ 查看生成的求购订单

DAY 1 下午
└─ 在悠悠有品手动创建求购

DAY 2 上午
├─ 物品到账
├─ 运行: ./bin/seller -target "物品" -price 100
└─ 物品已上架

DAY 2 下午
├─ 运行: ./bin/daemon
└─ 后台进程自动监控 (5分钟检查一次)

DAY 3+
└─ 后台进程继续运行,自动调整策略
```

---

## ❓ 常见问题

**Q: 如何停止daemon?**
A: `Ctrl+C` 或 `pkill -f "bin/daemon"`

**Q: daemon多久检查一次?**
A: 默认5分钟,可用 `-interval` 调整

**Q: 三个脚本可以同时运行吗?**
A: 可以,完全独立

**Q: 能否修改STEAM_ID?**
A: 需要编辑源代码后重新编译

---

## 🚀 立即开始

```bash
# 查看命令参考
cat COMMANDS.txt

# 或直接运行分析脚本
./bin/analyzer -budget 50
```

---

**准备好了？运行第一个命令吧！** 👉 `./bin/analyzer -budget 50`
