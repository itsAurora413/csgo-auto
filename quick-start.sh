#!/bin/bash

# CSGO2 自动交易平台 - 快速启动脚本（开发模式）
# 适用于开发环境，不进行构建，直接启动

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo
echo "================================================"
echo -e "${GREEN}  CSGO2 自动交易平台 - 快速启动${NC}"
echo "================================================"
echo

# 切换到脚本目录
cd "$(dirname "$0")"

# 检查必要文件
if [ ! -f "main.go" ]; then
    echo -e "${RED}错误: 未找到main.go文件${NC}"
    exit 1
fi

# 清理之前的进程
echo -e "${BLUE}[INFO]${NC} 清理之前的进程..."
pkill -f "go run main.go" 2>/dev/null || true
pkill -f "npm start" 2>/dev/null || true
sleep 2

# 创建必要的目录
mkdir -p logs

# 检查配置
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}[WARNING]${NC} 创建默认.env文件"
    cp .env.example .env 2>/dev/null || cat > .env << EOF
DATABASE_URL=csgo_trader.db
STEAM_API_KEY=your_steam_api_key_here
PORT=8080
ENVIRONMENT=development
EOF
fi

# 安装依赖（如果需要）
if [ ! -d "node_modules" ] && [ -d "web" ]; then
    echo -e "${BLUE}[INFO]${NC} 安装前端依赖..."
    cd web
    npm install
    cd ..
fi

# 启动后端（开发模式）
echo -e "${BLUE}[INFO]${NC} 启动后端服务 (开发模式)..."
go mod tidy 2>/dev/null || true
nohup go run main.go > logs/backend-dev.log 2>&1 &
BACKEND_PID=$!

# 等待后端启动
sleep 5

# 检查后端是否成功启动
if ! ps -p $BACKEND_PID > /dev/null; then
    echo -e "${RED}[ERROR]${NC} 后端启动失败，请检查日志: logs/backend-dev.log"
    exit 1
fi

echo -e "${GREEN}[SUCCESS]${NC} 后端服务启动成功!"

# 启动前端（开发模式）
if [ -d "web" ]; then
    echo -e "${BLUE}[INFO]${NC} 启动前端开发服务器..."
    cd web
    nohup npm start > ../logs/frontend-dev.log 2>&1 &
    FRONTEND_PID=$!
    cd ..

    echo -e "${GREEN}[SUCCESS]${NC} 前端开发服务器启动中..."
    echo -e "${YELLOW}[INFO]${NC} 前端通常在 http://localhost:3000"
fi

echo
echo "================================================"
echo -e "${GREEN}🚀 开发服务已启动!${NC}"
echo "================================================"
echo
echo -e "🔧 后端API: ${BLUE}http://localhost:8080/api/v1${NC}"
echo -e "📊 健康检查: ${BLUE}http://localhost:8080/health${NC}"
if [ -d "web" ]; then
echo -e "📱 前端开发: ${BLUE}http://localhost:3000${NC} (热重载)"
fi
echo
echo -e "📁 后端日志: ${YELLOW}logs/backend-dev.log${NC}"
if [ -d "web" ]; then
echo -e "📁 前端日志: ${YELLOW}logs/frontend-dev.log${NC}"
fi
echo
echo -e "${YELLOW}按 Ctrl+C 停止所有服务${NC}"

# 捕获中断信号
trap 'echo; echo "正在停止服务..."; kill $BACKEND_PID 2>/dev/null || true; [ ! -z "$FRONTEND_PID" ] && kill $FRONTEND_PID 2>/dev/null || true; pkill -f "go run main.go" 2>/dev/null || true; pkill -f "npm start" 2>/dev/null || true; echo "所有服务已停止"; exit 0' INT TERM

# 等待
while true; do
    sleep 1
done
