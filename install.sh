#!/bin/bash
# CHROTE Installer
# Usage: curl -sL https://raw.githubusercontent.com/Perttulands/CHROTE/main/install.sh | bash
set -e

REPO="Perttulands/CHROTE"
INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.chrote"
SERVICE_DIR="$HOME/.config/systemd/user"
TTYD_VERSION="1.7.7"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log() { echo -e "${CYAN}[CHROTE]${NC} $1"; }
success() { echo -e "${GREEN}[CHROTE]${NC} $1"; }
warn() { echo -e "${YELLOW}[CHROTE]${NC} $1"; }
error() { echo -e "${RED}[CHROTE]${NC} $1"; exit 1; }

# 1. Detect environment
detect_env() {
    if grep -qi microsoft /proc/version 2>/dev/null; then
        echo "wsl"
    else
        echo "linux"
    fi
}

# 2. Check prerequisites
check_prereqs() {
    log "Checking prerequisites..."

    # Check for curl
    if ! command -v curl &>/dev/null; then
        error "curl is required but not installed. Install with: sudo apt install curl"
    fi

    # Check for tmux
    if ! command -v tmux &>/dev/null; then
        warn "tmux not found. Installing..."
        if command -v apt-get &>/dev/null; then
            sudo apt-get update && sudo apt-get install -y tmux
        elif command -v brew &>/dev/null; then
            brew install tmux
        else
            error "Cannot install tmux. Please install manually."
        fi
    fi

    success "Prerequisites OK"
}

# 3. Create directories
setup_dirs() {
    log "Setting up directories..."
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$SERVICE_DIR"

    # Ensure ~/.local/bin is in PATH
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
        warn "Adding ~/.local/bin to PATH in ~/.bashrc"
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
    fi

    success "Directories created"
}

# 4. Download chrote-server binary
download_binary() {
    log "Downloading chrote-server..."

    # Get latest release
    LATEST=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)

    if [ -z "$LATEST" ]; then
        warn "No releases found. Building from source..."
        build_from_source
        return
    fi

    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac

    BINARY_URL="https://github.com/$REPO/releases/download/$LATEST/chrote-server-linux-$ARCH"

    if curl -sL --fail "$BINARY_URL" -o "$INSTALL_DIR/chrote-server" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/chrote-server"
        success "Downloaded chrote-server $LATEST"
    else
        warn "Binary not found in release. Building from source..."
        build_from_source
    fi
}

# 5. Build from source (fallback)
build_from_source() {
    log "Building from source..."

    # Check for Go
    if ! command -v go &>/dev/null; then
        error "Go is required to build from source. Install from https://go.dev/dl/"
    fi

    # Check for Node.js
    if ! command -v node &>/dev/null; then
        error "Node.js is required to build from source. Install from https://nodejs.org/"
    fi

    # Clone repo
    TEMP_DIR=$(mktemp -d)
    git clone --depth 1 "https://github.com/$REPO.git" "$TEMP_DIR/chrote"

    # Build dashboard
    log "Building dashboard..."
    cd "$TEMP_DIR/chrote/dashboard"
    npm ci
    npm run build
    cp -r dist ../src/internal/dashboard/

    # Build Go server
    log "Building server..."
    cd "$TEMP_DIR/chrote/src"
    go build -o "$INSTALL_DIR/chrote-server" ./cmd/server

    # Cleanup
    rm -rf "$TEMP_DIR"

    success "Built chrote-server from source"
}

# 6. Download ttyd
download_ttyd() {
    log "Downloading ttyd..."

    ARCH=$(uname -m)
    TTYD_URL="https://github.com/tsl0922/ttyd/releases/download/$TTYD_VERSION/ttyd.$ARCH"

    if curl -sL --fail "$TTYD_URL" -o "$INSTALL_DIR/ttyd" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/ttyd"
        success "Downloaded ttyd $TTYD_VERSION"
    else
        warn "Could not download ttyd. Terminal feature may not work."
    fi
}

# 7. Setup workspace
setup_workspace() {
    log "Setting up workspace..."

    # Default workspace
    WORKSPACE="${CHROTE_WORKSPACE:-$HOME/chrote-workspace}"

    if [ ! -d "$WORKSPACE" ]; then
        mkdir -p "$WORKSPACE"
        success "Created workspace at $WORKSPACE"
    fi

    # Create config
    cat > "$CONFIG_DIR/config.yaml" << EOF
# CHROTE Configuration
server:
  port: 8080

filesystem:
  allowed_roots:
    - $WORKSPACE
EOF

    success "Config created at $CONFIG_DIR/config.yaml"
}

# 8. Setup systemd service (user mode)
setup_systemd() {
    log "Setting up systemd service..."

    # Check if systemd is available
    if ! command -v systemctl &>/dev/null; then
        warn "systemd not available. Skipping service setup."
        warn "You can start CHROTE manually with: chrote-server"
        return
    fi

    # Create service file
    cat > "$SERVICE_DIR/chrote.service" << EOF
[Unit]
Description=CHROTE Server
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/chrote-server
Restart=on-failure
RestartSec=5
Environment=TMUX_TMPDIR=%t/tmux
Environment=PORT=8080

[Install]
WantedBy=default.target
EOF

    # Create ttyd service
    cat > "$SERVICE_DIR/chrote-ttyd.service" << EOF
[Unit]
Description=CHROTE Web Terminal (ttyd)
After=network.target

[Service]
Type=simple
ExecStartPre=/bin/mkdir -p %t/tmux
ExecStart=$INSTALL_DIR/ttyd -p 7681 -W bash
Restart=on-failure
RestartSec=5
Environment=TMUX_TMPDIR=%t/tmux

[Install]
WantedBy=default.target
EOF

    # Reload and enable
    systemctl --user daemon-reload
    systemctl --user enable chrote chrote-ttyd 2>/dev/null || true

    success "Systemd services configured"
}

# Main
main() {
    echo ""
    echo -e "${CYAN}================================${NC}"
    echo -e "${CYAN}   CHROTE Installer${NC}"
    echo -e "${CYAN}================================${NC}"
    echo ""

    ENV=$(detect_env)
    log "Detected environment: $ENV"

    check_prereqs
    setup_dirs
    download_binary
    download_ttyd
    setup_workspace
    setup_systemd

    echo ""
    echo -e "${GREEN}================================${NC}"
    echo -e "${GREEN}   Installation Complete!${NC}"
    echo -e "${GREEN}================================${NC}"
    echo ""
    echo "To start CHROTE:"
    echo "  systemctl --user start chrote chrote-ttyd"
    echo ""
    echo "To enable auto-start:"
    echo "  systemctl --user enable chrote chrote-ttyd"
    echo ""
    echo "Dashboard: http://localhost:8080"
    echo ""
    echo "Note: You may need to restart your shell or run:"
    echo "  source ~/.bashrc"
    echo ""
}

main "$@"
