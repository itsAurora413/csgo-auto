#!/bin/bash

# CSQAQ Standalone Sampler Package Script

set -e

VERSION="v1.0.0"
PACKAGE_NAME="csqaq-sampler-${VERSION}"

echo "📦 Packaging CSQAQ Standalone Sampler ${VERSION}..."

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Create package directory
log_info "Creating package directory..."
rm -rf dist
mkdir -p dist/${PACKAGE_NAME}

# Copy binaries
log_info "Copying binaries..."
cp csqaq-sampler-linux-amd64 dist/${PACKAGE_NAME}/
cp csqaq-sampler-linux-arm64 dist/${PACKAGE_NAME}/

# Copy configuration files
log_info "Copying configuration files..."
cp .env.example dist/${PACKAGE_NAME}/
cp README.md dist/${PACKAGE_NAME}/
cp Dockerfile dist/${PACKAGE_NAME}/
cp docker-compose.yml dist/${PACKAGE_NAME}/

# Create start script
log_info "Creating start script..."
cat > dist/${PACKAGE_NAME}/start.sh << 'EOF'
#!/bin/bash

# CSQAQ Sampler Startup Script

# Detect architecture
ARCH=$(uname -m)
BINARY=""

case $ARCH in
    x86_64)
        BINARY="./csqaq-sampler-linux-amd64"
        ;;
    aarch64|arm64)
        BINARY="./csqaq-sampler-linux-arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Detected architecture: $ARCH"
echo "Using binary: $BINARY"

# Check if .env exists
if [ ! -f ".env" ]; then
    echo "Warning: .env file not found. Copying from .env.example..."
    cp .env.example .env
    echo "Please edit .env file with your configuration before running again."
    exit 1
fi

# Make binary executable
chmod +x $BINARY

# Run the sampler
echo "Starting CSQAQ Sampler..."
$BINARY
EOF

chmod +x dist/${PACKAGE_NAME}/start.sh

# Create installation guide
log_info "Creating installation guide..."
cat > dist/${PACKAGE_NAME}/INSTALL.md << 'EOF'
# CSQAQ Sampler Installation Guide

## Quick Install

1. **Extract the package**
   ```bash
   tar -xzf csqaq-sampler-v1.0.0.tar.gz
   cd csqaq-sampler-v1.0.0
   ```

2. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env with your database and API settings
   nano .env
   ```

3. **Run the sampler**
   ```bash
   ./start.sh
   ```

## Manual Installation

1. **Choose the correct binary for your system:**
   - Linux x86_64: `csqaq-sampler-linux-amd64`
   - Linux ARM64: `csqaq-sampler-linux-arm64`

2. **Make it executable and run:**
   ```bash
   chmod +x csqaq-sampler-linux-amd64
   ./csqaq-sampler-linux-amd64
   ```

## Docker Installation

1. **Build and run with Docker:**
   ```bash
   docker build -t csqaq-sampler .
   docker run -d --name csqaq-sampler --env-file .env csqaq-sampler
   ```

2. **Or use Docker Compose:**
   ```bash
   docker-compose up -d
   ```

## Configuration

Edit `.env` file with your settings:

```env
DATABASE_URL=root:password@tcp(mysql-host:3306)/csgo_trader?charset=utf8mb4&parseTime=True&loc=Local
CSQAQ_API_KEY=your-api-key-here
ENVIRONMENT=production
```

## Service Installation (Systemd)

Create a systemd service file:

```bash
sudo tee /etc/systemd/system/csqaq-sampler.service > /dev/null << 'SYSTEMD_EOF'
[Unit]
Description=CSQAQ Price Sampler
After=network.target

[Service]
Type=simple
User=csqaq
WorkingDirectory=/opt/csqaq-sampler
ExecStart=/opt/csqaq-sampler/start.sh
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SYSTEMD_EOF

sudo systemctl daemon-reload
sudo systemctl enable csqaq-sampler
sudo systemctl start csqaq-sampler
```

For more details, see README.md
EOF

# Create archive
log_info "Creating archive..."
cd dist
tar -czf ${PACKAGE_NAME}.tar.gz ${PACKAGE_NAME}/
cd ..

# Show package info
log_success "Package created successfully!"
echo ""
echo "📦 Package: dist/${PACKAGE_NAME}.tar.gz"
echo "📁 Contents:"
ls -la dist/${PACKAGE_NAME}/
echo ""
echo "📊 Package size:"
du -h dist/${PACKAGE_NAME}.tar.gz
echo ""
echo "🚀 To deploy:"
echo "1. Upload dist/${PACKAGE_NAME}.tar.gz to your server"
echo "2. Extract: tar -xzf ${PACKAGE_NAME}.tar.gz"
echo "3. Configure: cd ${PACKAGE_NAME} && cp .env.example .env"
echo "4. Edit .env with your settings"
echo "5. Run: ./start.sh"