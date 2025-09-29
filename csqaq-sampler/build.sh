#!/bin/bash

# CSQAQ Standalone Sampler Build Script

set -e

echo "🚀 Building CSQAQ Standalone Sampler..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
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

# Check if Go is installed
if ! command -v go &> /dev/null; then
    log_error "Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
log_info "Go version: $GO_VERSION"

# Build for current platform
log_info "Building for current platform..."
go mod tidy
go build -o csqaq-sampler .
log_success "Built csqaq-sampler (current platform)"

# Build for Linux x86_64
log_info "Building for Linux x86_64..."
GOOS=linux GOARCH=amd64 go build -o csqaq-sampler-linux-amd64 .
log_success "Built csqaq-sampler-linux-amd64"

# Build for Linux ARM64
log_info "Building for Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -o csqaq-sampler-linux-arm64 .
log_success "Built csqaq-sampler-linux-arm64"

# Make binaries executable
chmod +x csqaq-sampler*

log_success "Build completed! Available binaries:"
ls -la csqaq-sampler*

echo ""
echo "📦 To deploy:"
echo "1. Copy the appropriate binary to your target server"
echo "2. Copy .env.example to .env and configure your settings"
echo "3. Run: ./csqaq-sampler-linux-amd64"
echo ""
echo "🐳 To build Docker image:"
echo "docker build -t csqaq-sampler ."
echo ""
echo "🚀 To run with Docker Compose:"
echo "docker-compose up -d"