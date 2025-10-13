# 跨平台构建说明

## 构建脚本功能

修改后的 `build.sh` 脚本支持为多个平台生成二进制文件，包括：

- **Linux x86_64** (默认)
- **Linux 32位**
- **macOS Intel**
- **macOS Apple Silicon (M1/M2)**
- **Windows 64位**

## 使用方法

### 1. 构建Linux x86_64版本（默认）
```bash
./build.sh
# 或
./build.sh linux
```

### 2. 构建其他平台
```bash
# Linux 32位
./build.sh linux-386

# macOS Intel
./build.sh darwin

# macOS Apple Silicon
./build.sh darwin-arm64

# Windows 64位
./build.sh windows
```

### 3. 构建所有平台
```bash
./build.sh all
```

## 生成的文件

构建完成后会生成以下文件：

```
bulk_fetch_goods_optimized-linux-amd64      # Linux x86_64
bulk_fetch_goods_optimized-linux-386        # Linux 32位
bulk_fetch_goods_optimized-darwin-amd64     # macOS Intel
bulk_fetch_goods_optimized-darwin-arm64     # macOS Apple Silicon
bulk_fetch_goods_optimized-windows-amd64.exe # Windows 64位
```

## 在Linux服务器上运行

### 1. 上传文件
将 `bulk_fetch_goods_optimized-linux-amd64` 上传到您的Linux服务器。

### 2. 添加执行权限
```bash
chmod +x bulk_fetch_goods_optimized-linux-amd64
```

### 3. 运行程序
```bash
# 普通IP模式（1秒/次）
./bulk_fetch_goods_optimized-linux-amd64

# 绑定IP模式（30秒/次）
./bulk_fetch_goods_optimized-linux-amd64 -binding

# 自定义普通IP间隔（毫秒）
./bulk_fetch_goods_optimized-linux-amd64 1500  # 1.5秒间隔
```

## 构建特性

### 1. 静态链接
- 使用 `CGO_ENABLED=0` 生成静态链接的二进制文件
- 不依赖系统库，可以在任何Linux发行版上运行

### 2. 优化编译
- 使用 `-ldflags="-s -w"` 去除调试信息，减小文件大小
- 添加构建时间戳版本信息

### 3. 跨平台兼容
- 支持主流操作系统和架构
- 自动处理不同平台的文件扩展名（Windows .exe）

## 文件大小对比

| 平台 | 文件大小 | 说明 |
|------|----------|------|
| Linux x86_64 | ~6.0MB | 推荐用于服务器 |
| Linux 32位 | ~5.9MB | 兼容老系统 |
| macOS Intel | ~6.2MB | Intel Mac |
| macOS ARM64 | ~5.8MB | M1/M2 Mac |
| Windows 64位 | ~6.2MB | Windows系统 |

## 验证二进制文件

### Linux
```bash
file bulk_fetch_goods_optimized-linux-amd64
# 输出: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked
```

### macOS
```bash
file bulk_fetch_goods_optimized-darwin-amd64
# 输出: Mach-O 64-bit executable x86_64
```

### Windows
```bash
file bulk_fetch_goods_optimized-windows-amd64.exe
# 输出: PE32+ executable (console) x86-64, for MS Windows
```

## 部署建议

### 1. 服务器部署
- 推荐使用 `bulk_fetch_goods_optimized-linux-amd64`
- 确保服务器有足够的磁盘空间和网络带宽
- 建议在screen或tmux会话中运行长期任务

### 2. 本地测试
- 使用对应平台的二进制文件进行本地测试
- 验证数据库连接和API访问正常

### 3. 监控和日志
- 程序会输出详细的运行日志
- 建议将日志重定向到文件：
```bash
./bulk_fetch_goods_optimized-linux-amd64 > app.log 2>&1 &
```

这样修改后的构建脚本让您能够轻松地为不同平台生成优化的二进制文件，特别是您需要的Linux x86_64版本。
