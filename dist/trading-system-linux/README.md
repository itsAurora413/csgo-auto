# 三脚本交易系统 - Linux x86_64 版本

## 快速开始

### 1. 设置权限
```bash
chmod +x analyzer seller daemon
```

### 2. 运行分析脚本
```bash
./analyzer -budget 50
```

### 3. 运行出售脚本 (需要私钥)
```bash
export YOUPIN_PRIVATE_KEY="$(cat private_key.pem)"
./seller -target "P250 | 污染物" -price 23.00
```

### 4. 启动后台守护进程
```bash
./daemon
```

## 文件说明

- `analyzer` - 分析脚本 (发现机会 + 生成订单)
- `seller` - 出售脚本 (上架 + 设置价格)
- `daemon` - 守护进程 (监控 + 止损/止盈 + 调整 + 回测)

## 文档

- `START_HERE.md` - 快速入门 (推荐首先阅读)
- `COMMANDS.txt` - 命令参考
- `NEW_WORKFLOW.md` - 详细流程
- `REFACTOR_SUMMARY.md` - 技术细节

## 硬编码配置

所有脚本都包含以下硬编码配置:
- STEAM_ID = 76561199078507841
- YOUPIN_APP_KEY = 12919014

seller 脚本需要环境变量:
- export YOUPIN_PRIVATE_KEY="$(cat private_key.pem)"

## 系统要求

- Linux x86_64
- glibc 2.17+
- 网络连接 (连接远程数据库)

## 常见问题

**Q: 如何停止 daemon?**
A: `Ctrl+C` 或 `pkill -f "daemon"`

**Q: daemon 多久检查一次?**
A: 默认 5 分钟,可用 `-interval` 参数自定义

**Q: 需要 Go 环境吗?**
A: 不需要,这是已编译的二进制文件

## 后台运行

在后台持续运行 daemon (关闭终端后继续运行):
```bash
nohup ./daemon > daemon.log 2>&1 &
```

查看日志:
```bash
tail -f daemon.log
```

## 版本信息

编译时间: 2025-10-18
编译平台: Linux x86_64
Go 版本: 1.21+
