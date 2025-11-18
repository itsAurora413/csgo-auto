#!/bin/bash

# CSGO2 自动交易平台 - 一键运行脚本
# 此脚本会自动检查环境、安装依赖、构建并启动前后端服务

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查命令是否存在
check_command() {
    if ! command -v "$1" &> /dev/null; then
        return 1
    fi
    return 0
}

# 检查端口是否被占用
is_port_busy() {
    local port="$1"
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

# 寻找可用端口
find_available_port() {
    local base_port="${1:-8080}"
    local max_attempts=20
    local p=$base_port
    for _ in $(seq 1 $max_attempts); do
        if ! is_port_busy "$p"; then
            echo "$p"
            return 0
        fi
        p=$((p+1))
    done
    return 1
}

# 停止之前的进程
cleanup_processes() {
    log_info "清理之前的进程..."
    pkill -f "csgo-trader" 2>/dev/null || true
    pkill -f "python.*main.py" 2>/dev/null || true
    pkill -f "npm.*start" 2>/dev/null || true
    sleep 2
}

# 显示欢迎信息
show_welcome() {
    echo
    echo "================================================"
    echo -e "${GREEN}  CSGO2 自动交易平台 - 一键运行脚本${NC}"
    echo "================================================"
    echo
    log_info "此脚本将自动："
    echo "  ✓ 检查环境依赖"
    echo "  ✓ 安装必要的软件包"
    echo "  ✓ 构建前后端"
    echo "  ✓ 启动所有服务"
    echo
}

# 检查环境依赖
check_prerequisites() {
    log_info "检查环境依赖..."

    local missing_deps=()

    # 检查Go
    if ! check_command "go"; then
        missing_deps+=("Go (https://golang.org/dl/)")
    else
        local go_version=$(go version | grep -o 'go[0-9]\+\.[0-9]\+' | head -1)
        log_success "Go已安装: $go_version"
    fi

    # 检查Node.js
    if ! check_command "node"; then
        missing_deps+=("Node.js (https://nodejs.org/)")
    else
        local node_version=$(node --version)
        log_success "Node.js已安装: $node_version"
    fi

    # 检查npm
    if ! check_command "npm"; then
        missing_deps+=("npm (通常随Node.js安装)")
    else
        local npm_version=$(npm --version)
        log_success "npm已安装: $npm_version"
    fi

    # 检查Python
    if ! check_command "python3"; then
        missing_deps+=("Python3")
    else
        local python_version=$(python3 --version)
        log_success "Python3已安装: $python_version"
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "缺少以下依赖："
        for dep in "${missing_deps[@]}"; do
            echo "  - $dep"
        done
        echo
        log_error "请安装缺少的依赖后重新运行此脚本"
        exit 1
    fi

    log_success "所有环境依赖检查通过!"
}

# 安装Go依赖
install_go_deps() {
    log_info "安装Go依赖..."
    if [ -f "go.mod" ]; then
        go mod tidy
        log_success "Go依赖安装完成"
    else
        log_warning "未找到go.mod文件"
    fi
}

# 安装前端依赖
install_frontend_deps() {
    log_info "安装前端依赖..."
    if [ -d "web" ]; then
        cd web
        if [ -f "package.json" ]; then
            log_info "正在运行 npm install..."
            npm install
            log_success "前端依赖安装完成"
        else
            log_warning "web目录中未找到package.json文件"
        fi
        cd ..
    else
        log_warning "未找到web目录"
    fi
}

# 构建后端
build_backend() {
    log_info "构建Go后端..."
    if [ -f "main.go" ]; then
        go build -o csgo-trader .
        log_success "后端构建完成"
    else
        log_error "未找到main.go文件"
        exit 1
    fi
}

# 构建前端
build_frontend() {
    log_info "构建React前端..."
    if [ -d "web" ]; then
        cd web
        if [ -f "package.json" ]; then
            log_info "正在运行 npm run build..."
            npm run build
            log_success "前端构建完成"
        else
            log_error "web目录中未找到package.json文件"
            exit 1
        fi
        cd ..
    else
        log_error "未找到web目录"
        exit 1
    fi
}

# 检查配置文件
check_config() {
    log_info "检查配置文件..."

    if [ ! -f ".env" ]; then
        if [ -f ".env.example" ]; then
            log_warning ".env文件不存在，从.env.example创建"
            cp .env.example .env
        else
            log_warning "创建默认.env文件"
            cat > .env << EOF
# CSGO2 Auto Trading Platform - Environment Configuration
DATABASE_URL=csgo_trader.db
STEAM_API_KEY=your_steam_api_key_here
PORT=8080
ENVIRONMENT=development
LOG_LEVEL=INFO

# YouPin Open API Configuration
YOUPIN_USE_OPEN_API=true
YOUPIN_APP_KEY=your_app_key_here
YOUPIN_PRIVATE_KEY=your_private_key_here
YOUPIN_CALLBACK_PUBLIC_KEY=your_callback_public_key_here
EOF
        fi
    fi

    # 检查Steam API Key
    if grep -q "your_steam_api_key_here" .env; then
        log_warning "请在.env文件中配置你的Steam API Key"
        log_warning "获取地址: https://steamcommunity.com/dev/apikey"
    fi

    # 检查并加载悠悠有品开放平台配置
    log_info "检查悠悠有品开放平台配置..."
    source .env

    # 验证开放平台配置（必须配置）
    if [ -z "$YOUPIN_APP_KEY" ] || [ "$YOUPIN_APP_KEY" = "your_app_key_here" ]; then
        log_error "❌ 悠悠有品AppKey未配置"
        log_error "请在.env中配置 YOUPIN_APP_KEY"
        log_error "获取方式：https://open.youpin898.com"
        exit 1
    fi

    if [ -z "$YOUPIN_PRIVATE_KEY" ] || [ "$YOUPIN_PRIVATE_KEY" = "your_private_key_here" ]; then
        log_error "❌ 悠悠有品私钥未配置"
        log_error "请在.env中配置 YOUPIN_PRIVATE_KEY"
        log_error "生成方式：go run cmd/test-youpin-openapi/main.go generate-keys"
        exit 1
    fi

    log_success "✅ 悠悠有品开放平台API已配置"
    log_info "AppKey: ${YOUPIN_APP_KEY:0:8}..."

    # 导出环境变量供后端使用
    export YOUPIN_USE_OPEN_API="true"
    export YOUPIN_APP_KEY="$YOUPIN_APP_KEY"
    export YOUPIN_PRIVATE_KEY="$YOUPIN_PRIVATE_KEY"
    export YOUPIN_CALLBACK_PUBLIC_KEY="$YOUPIN_CALLBACK_PUBLIC_KEY"

    log_success "配置文件检查完成"
}

# 启动服务
start_services() {
    log_info "启动服务..."

    # 检查端口
    local port="${PORT:-8080}"
    if is_port_busy "$port"; then
        local new_port
        new_port=$(find_available_port "$port")
        if [ $? -eq 0 ]; then
            log_warning "端口 $port 被占用，使用端口 $new_port"
            export PORT="$new_port"
            port="$new_port"
        else
            log_error "无法找到可用端口"
            exit 1
        fi
    fi

    # 创建日志目录
    mkdir -p logs

    # 启动后端
    log_info "启动后端服务..."
    if [ -f "./csgo-trader" ]; then
        chmod +x ./csgo-trader
        nohup ./csgo-trader > logs/backend.log 2>&1 &
        BACKEND_PID=$!
        sleep 3

        # 检查后端是否启动成功
        if ps -p $BACKEND_PID > /dev/null; then
            log_success "后端服务启动成功 (PID: $BACKEND_PID)"
        else
            log_error "后端服务启动失败，请检查日志: logs/backend.log"
            exit 1
        fi
    else
        log_error "未找到csgo-trader可执行文件"
        exit 1
    fi

    # 等待服务就绪
    log_info "等待服务就绪..."
    for i in {1..30}; do
        if curl -s "http://localhost:$port/health" > /dev/null 2>&1; then
            break
        fi
        if [ $i -eq 30 ]; then
            log_error "服务启动超时"
            exit 1
        fi
        sleep 1
    done

    log_success "所有服务启动完成!"
    echo
    echo "================================================"
    echo -e "${GREEN}🚀 CSGO2 自动交易平台已启动!${NC}"
    echo "================================================"
    echo
    echo -e "📱 Web界面: ${BLUE}http://localhost:$port${NC}"
    echo -e "🔧 后端API: ${BLUE}http://localhost:$port/api/v1${NC}"
    echo -e "📊 健康检查: ${BLUE}http://localhost:$port/health${NC}"
    echo
    echo -e "📁 日志文件: ${YELLOW}logs/backend.log${NC}"
    echo -e "⚙️  配置文件: ${YELLOW}.env${NC}"
    echo
    echo -e "${YELLOW}按 Ctrl+C 停止所有服务${NC}"
    echo
}

# 等待中断信号
wait_for_interrupt() {
    # 捕获中断信号
    trap 'echo; log_info "正在停止服务..."; cleanup_processes; log_success "所有服务已停止"; exit 0' INT TERM

    # 持续监控后端进程
    while true; do
        if ! ps -p $BACKEND_PID > /dev/null 2>&1; then
            log_error "后端进程意外退出，请检查日志"
            break
        fi
        sleep 5
    done
}

# 主函数
main() {
    # 确保在脚本目录中运行
    cd "$(dirname "$0")"

    show_welcome

    # 清理之前的进程
    cleanup_processes

    # 检查环境
    check_prerequisites

    # 检查配置
    check_config

    # 安装依赖
    install_go_deps
    install_frontend_deps

    # 构建项目
    build_backend
    build_frontend

    # 启动服务
    start_services

    # 等待中断
    wait_for_interrupt
}

# 运行主函数
main "$@"