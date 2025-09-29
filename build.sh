#!/bin/bash

# CSGO2 Auto Trading Platform - Linux/macOS Build Script
# This script builds the entire application for deployment

set -e

echo "===================================="
echo "CSGO2 Auto Trading Platform Builder"
echo "===================================="
echo

# Check prerequisites
echo "Checking prerequisites..."

# Check Go
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

# Check Node.js
if ! command -v node &> /dev/null; then
    echo "ERROR: Node.js is not installed"
    echo "Please install Node.js from https://nodejs.org/"
    exit 1
fi

# Check Python
if ! command -v python3 &> /dev/null; then
    echo "ERROR: Python 3 is not installed"
    echo "Please install Python 3"
    exit 1
fi

echo "All prerequisites found!"
echo

# Create build directory
mkdir -p build/{logs,data}

echo "Building Go backend..."
echo

# Build Go backend (use host env to avoid CGO cross-compile issues)
go mod tidy
go build -o build/csgo-trader .

echo "Go backend built successfully!"
echo

echo "Installing Python dependencies (virtualenv)..."
echo

# Setup Python virtual environment to avoid system-managed restrictions (PEP 668)
PY_CMD="python3"
if command -v python3.11 >/dev/null 2>&1; then
    PY_CMD="python3.11"
fi

"$PY_CMD" -m venv build/venv
source build/venv/bin/activate
python -m pip install --upgrade pip

# Prefer binary wheels to avoid building heavy packages under newer Python
set +e
pip install --only-binary=:all: -r requirements.txt
PIP_STATUS=$?
set -e

if [ $PIP_STATUS -ne 0 ]; then
    echo "Binary wheels install failed; retrying with lightweight set (skipping pandas/numpy/matplotlib)."
    grep -Ev '^(pandas|numpy|matplotlib)=' requirements.txt > build/requirements-lite.txt || true
    pip install -r build/requirements-lite.txt
    echo "WARNING: Skipped heavy scientific packages due to Python version/wheel availability."
fi

echo "Python dependencies installed!"
echo

echo "Building React frontend..."
echo

# Build React frontend
cd web
npm install
npm run build
cd ..

# Copy built frontend
mkdir -p build/web
# Ensure target matches backend static path defaults
rm -rf build/web/build
cp -R web/build build/web/build

echo "Frontend built and copied successfully!"
echo

# Copy Python files
echo "Copying Python data collector..."
cp -r python build/

# Copy configuration files
echo "Copying configuration files..."
cp .env.example build/.env
cp README.md build/

# Create start scripts
echo "Creating start scripts..."

# Create Linux start script
cat > build/start.sh << 'EOF'
#!/bin/bash

set -Eeuo pipefail

# Always run from this script's directory
SCRIPT_DIR="$(cd -- "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "Starting CSGO2 Auto Trading Platform..."
echo

# Load .env if present (export variables)
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    . ./.env || true
    set +a
fi

# Functions
is_port_busy() {
    local port="$1"
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

find_available_port() {
    local base_port="${PORT:-8080}"
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

# Ensure previous csgo-trader on same port is not running
TARGET_PORT="${PORT:-8080}"
if is_port_busy "$TARGET_PORT"; then
    # If the listener is csgo-trader, stop it; otherwise pick a new port
    if lsof -nP -iTCP:"$TARGET_PORT" -sTCP:LISTEN | grep -q "csgo-trader"; then
        echo "Detected previous csgo-trader on port $TARGET_PORT, stopping it..."
        # Try graceful then force
        pkill -f "csgo-trader" || true
        sleep 1
        if is_port_busy "$TARGET_PORT"; then
            pkill -9 -f "csgo-trader" || true
            sleep 1
        fi
    fi
fi

# If still busy (by other app), choose next available port
if is_port_busy "$TARGET_PORT"; then
    NEW_PORT="$(find_available_port)" || {
        echo "ERROR: No free port found near $TARGET_PORT"
        exit 1
    }
    echo "Port $TARGET_PORT is in use by another process. Using PORT=$NEW_PORT instead."
    export PORT="$NEW_PORT"
else
    export PORT="$TARGET_PORT"
fi

# Start data collector in background
if [ -x ./venv/bin/python ]; then
    ./venv/bin/python python/main.py &
else
    # Fallback to system python if venv is missing
    python3 python/main.py &
fi
PYTHON_PID=$!

# Wait a moment
sleep 2

# Start main application
./csgo-trader &
GO_PID=$!

echo "Both services started!"
echo "Python PID: $PYTHON_PID"
echo "Go PID: $GO_PID"
echo
echo "Access the application at http://localhost:${PORT}"
echo "Press Ctrl+C to stop all services"

# Wait for interrupt
trap 'echo "Stopping services..."; kill -0 "$PYTHON_PID" >/dev/null 2>&1 && kill "$PYTHON_PID" || true; kill -0 "$GO_PID" >/dev/null 2>&1 && kill "$GO_PID" || true; exit' INT
wait
EOF

# Create stop script
cat > build/stop.sh << 'EOF'
#!/bin/bash

echo "Stopping CSGO2 Auto Trading Platform..."

pkill -f csgo-trader
pkill -f "python.*main.py"

echo "Services stopped!"
EOF

# Make scripts executable
chmod +x build/start.sh
chmod +x build/stop.sh
chmod +x build/csgo-trader

# Create README for build
cat > build/BUILD_README.md << 'EOF'
# CSGO2 Auto Trading Platform

## Setup Instructions

1. Copy the .env file and configure your API keys:
   - STEAM_API_KEY: Get from https://steamcommunity.com/dev/apikey
   - BUFF_API_KEY: Get from BUFF163
   - YOUPIN_API_KEY: Get from YouPin898

2. Run start.sh to start the application:
   ```bash
   ./start.sh
   ```

3. Open http://localhost:8080 in your browser

4. Use stop.sh to stop all services:
   ```bash
   ./stop.sh
   ```

## File Structure

- csgo-trader: Main Go backend server
- python/: Data collection service
- web/: Frontend files
- logs/: Application logs
- data/: Database files

## Requirements

- Linux or macOS
- Python 3.7+
- Go 1.19+
- Node.js 16+
EOF

echo
echo "===================================="
echo "Build completed successfully!"
echo "===================================="
echo
echo "Build location: $(pwd)/build"
echo
echo "Next steps:"
echo "1. Navigate to the build directory"
echo "2. Configure your API keys in .env file"
echo "3. Run ./start.sh to start the application"
echo "4. Open http://localhost:8080 in your browser"
echo