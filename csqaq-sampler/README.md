# CSQAQ Standalone Sampler

独立的CSQAQ价格采样服务，专门用于定时采集CSGO商品价格数据。

## 功能特性

- 🔄 **连续采样**: 每1.6秒采集一个商品的价格数据
- 📊 **多平台价格**: 同时采集YYYP和BUFF平台的买卖价格
- 🛡️ **错误处理**: 自动重试和错误恢复机制
- 📈 **进度追踪**: 实时显示采样进度和成功率
- 🚀 **高性能**: 优化的数据库操作和网络请求
- 🐳 **容器化**: 支持Docker部署

## 快速开始

### 方式一：直接运行

1. **配置环境**
   ```bash
   cp .env.example .env
   # 编辑 .env 文件，配置数据库和API密钥
   ```

2. **构建并运行**
   ```bash
   ./build.sh
   ./csqaq-sampler
   ```

### 方式二：Docker部署

1. **配置环境**
   ```bash
   cp .env.example .env
   # 编辑 .env 文件
   ```

2. **Docker构建**
   ```bash
   docker build -t csqaq-sampler .
   ```

3. **运行容器**
   ```bash
   docker run -d --name csqaq-sampler \
     --env-file .env \
     csqaq-sampler
   ```

### 方式三：Docker Compose（推荐）

1. **配置环境**
   ```bash
   cp .env.example .env
   # 编辑 .env 文件，配置以下变量：
   # DATABASE_URL=root:password@tcp(mysql:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local
   # CSQAQ_API_KEY=你的API密钥
   # MYSQL_ROOT_PASSWORD=强密码
   # MYSQL_USER=csqaq
   # MYSQL_PASSWORD=用户密码
   ```

2. **启动服务**
   ```bash
   docker-compose up -d
   ```

## 配置说明

### 环境变量

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `DATABASE_URL` | MySQL数据库连接字符串 | `root:password@tcp(mysql-host:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local` |
| `CSQAQ_API_KEY` | CSQAQ API密钥 | `WPXHV1H7O5Y8N8W6R8U1N249` |
| `ENVIRONMENT` | 运行环境 | `production` |

### 数据库要求

程序需要以下数据表：

- `csqaq_goods`: 商品基础信息
- `csqaq_good_snapshots`: 价格快照数据

确保数据库中存在这些表结构。

## 监控和日志

### 日志输出
程序会输出详细的运行日志：
```
2025/09/29 13:46:49 CSQAQ Standalone Sampler initialized successfully
2025/09/29 13:46:49 Using API Key: WPXH****N249
2025/09/29 13:46:49 Database connected: root:****@tcp(mysql:3306)/csgo_trader
[Enhanced CSQAQ Sampler] Starting continuous sampling with 1.6s intervals
[Enhanced CSQAQ Sampler] Loaded 1528 goods for continuous processing
Successfully bound local IP to CSQAQ API
[Enhanced CSQAQ Sampler] Progress: 1/1528 processed, 1 valid prices, 100.0% success rate
```

### 性能指标
- **采样间隔**: 1.6秒/商品
- **成功率**: 通常保持在90%以上
- **重试机制**: 自动重试失败的请求
- **IP绑定**: 每35秒自动重新绑定IP

## 部署建议

### 生产环境

1. **资源配置**
   - CPU: 1核心
   - 内存: 512MB
   - 存储: 10GB（日志和临时文件）

2. **网络要求**
   - 稳定的互联网连接
   - 访问CSQAQ API的权限
   - 访问MySQL数据库的权限

3. **监控建议**
   - 监控进程状态
   - 监控数据库连接
   - 监控API调用成功率
   - 设置日志轮转

### 故障处理

1. **常见问题**
   - API密钥无效：检查`.env`中的`CSQAQ_API_KEY`
   - 数据库连接失败：检查`DATABASE_URL`配置
   - 网络超时：检查网络连接和防火墙设置

2. **重启服务**
   ```bash
   # Docker Compose
   docker-compose restart csqaq-sampler

   # 直接运行
   pkill csqaq-sampler
   ./csqaq-sampler
   ```

## API接口

程序本身不提供HTTP API，它是一个纯粹的数据采集服务。采集的数据存储在MySQL数据库中，可以通过SQL查询或其他应用程序访问。

### 数据表结构

**csqaq_good_snapshots**
```sql
CREATE TABLE csqaq_good_snapshots (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    good_id BIGINT NOT NULL,
    yyyp_sell_price DECIMAL(10,6),
    yyyp_buy_price DECIMAL(10,6),
    buff_sell_price DECIMAL(10,6),
    buff_buy_price DECIMAL(10,6),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_good_id (good_id),
    INDEX idx_created_at (created_at)
);
```

## 开发说明

### 本地开发

1. **环境要求**
   - Go 1.21+
   - MySQL 8.0+
   - 有效的CSQAQ API密钥

2. **运行开发环境**
   ```bash
   go mod tidy
   cp .env.example .env
   # 配置 .env
   go run main.go
   ```

3. **构建说明**
   ```bash
   # 当前平台
   go build -o csqaq-sampler .

   # Linux x86_64
   GOOS=linux GOARCH=amd64 go build -o csqaq-sampler-linux-amd64 .

   # Linux ARM64
   GOOS=linux GOARCH=arm64 go build -o csqaq-sampler-linux-arm64 .
   ```

## 许可证

此项目仅供学习和研究使用。

## 支持

如有问题，请检查：
1. 日志输出中的错误信息
2. 网络连接和API密钥配置
3. 数据库连接状态