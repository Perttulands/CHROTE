#!/bin/bash
# CHROTE WSL Setup Script
# Run as root in a fresh Ubuntu 24.04 WSL instance
# Usage: sudo bash setup-wsl.sh
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() { echo -e "${GREEN}[CHROTE]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    error "Please run as root: sudo bash setup-wsl.sh"
fi

log "Starting CHROTE WSL setup..."

# ============================================================================
# Phase 1: System Configuration
# ============================================================================
log "Phase 1: System configuration..."

# Check if wsl.conf already configured
if ! grep -q "systemd = true" /etc/wsl.conf 2>/dev/null; then
    log "Configuring /etc/wsl.conf..."
    cat > /etc/wsl.conf << 'EOF'
[boot]
systemd = true

[user]
default = chrote

[automount]
enabled = true
options = "metadata,umask=22,fmask=11"

[interop]
enabled = true
appendWindowsPath = true
EOF
    warn "WSL config updated. You need to restart WSL after setup completes:"
    warn "  wsl --shutdown"
    warn "  wsl -d Ubuntu"
fi

# ============================================================================
# Phase 2: Create User
# ============================================================================
log "Phase 2: Creating chrote user..."

if id "chrote" &>/dev/null; then
    log "User 'chrote' already exists"
else
    useradd -m -s /bin/bash chrote
    passwd -l chrote
    log "Created user 'chrote' (no sudo, password locked)"
fi

# ============================================================================
# Phase 3: Create Directories
# ============================================================================
log "Phase 3: Creating directories..."

# Tmux socket directory (tmpfiles.d)
mkdir -p /etc/tmpfiles.d
cat > /etc/tmpfiles.d/chrote-tmux.conf << 'EOF'
d /run/tmux 0755 root root -
d /run/tmux/chrote 0700 chrote chrote -
EOF

# Create tmux directory now
mkdir -p /run/tmux/chrote
chown chrote:chrote /run/tmux/chrote
chmod 0700 /run/tmux/chrote

# Create symlinks
if [ ! -L /vault ]; then
    ln -s /mnt/e/Vault /vault 2>/dev/null || warn "/vault symlink not created (E: drive not mounted?)"
fi

# User directories
mkdir -p /home/chrote/.local/bin
chown -R chrote:chrote /home/chrote/.local

log "Directories created"

# ============================================================================
# Phase 4: Install Dependencies
# ============================================================================
log "Phase 4: Installing dependencies..."

apt-get update
apt-get install -y \
    curl git tmux python3 python3-pip \
    openssh-server locales build-essential \
    jq wget unzip

# Generate locale
locale-gen en_US.UTF-8

# Install Go
GO_VERSION="1.22.0"
if [ ! -d /usr/local/go ]; then
    log "Installing Go ${GO_VERSION}..."
    curl -LO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
    rm -rf /usr/local/go && tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
    rm "go${GO_VERSION}.linux-amd64.tar.gz"
else
    log "Go already installed"
fi

# Install Node.js 20 LTS
if ! command -v node &>/dev/null; then
    log "Installing Node.js 20..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
else
    log "Node.js already installed: $(node --version)"
fi

# Install ttyd
if [ ! -f /usr/local/bin/ttyd ]; then
    log "Installing ttyd..."
    curl -L "https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64" \
        -o /usr/local/bin/ttyd
    chmod +x /usr/local/bin/ttyd
else
    log "ttyd already installed"
fi

# Install Claude Code
if ! command -v claude &>/dev/null; then
    log "Installing Claude Code..."
    npm install -g @anthropic-ai/claude-code
else
    log "Claude Code already installed"
fi

log "Dependencies installed"

# ============================================================================
# Phase 5: Clone and Build (as chrote user)
# ============================================================================
log "Phase 5: Copying and building..."

CHROTE_HOME="/home/chrote/chrote"
WINDOWS_SOURCE="/mnt/e/Docker/AgentArena"

if [ ! -d "$CHROTE_HOME" ]; then
    if [ -d "$WINDOWS_SOURCE" ]; then
        log "Copying from Windows source: $WINDOWS_SOURCE"
        mkdir -p "$CHROTE_HOME"
        # Use rsync to exclude problematic files, or cp with error handling
        rsync -a --exclude='.beads/daemon.*' --exclude='node_modules' --exclude='.git' \
            "$WINDOWS_SOURCE/" "$CHROTE_HOME/" 2>/dev/null || \
            cp -r "$WINDOWS_SOURCE"/* "$CHROTE_HOME/" 2>/dev/null || true
        chown -R chrote:chrote "$CHROTE_HOME"
    else
        log "Cloning from GitHub..."
        su - chrote -c "git clone https://github.com/Perttulands/CHROTE.git ~/chrote"
    fi
fi

# Create /code symlink
if [ ! -L /code ]; then
    ln -s /home/chrote/chrote /code
    chown -h chrote:chrote /code
fi

# Build dashboard
log "Building dashboard..."
su - chrote -c "cd ~/chrote/dashboard && npm ci && npm run build"

# Copy dashboard to Go server
su - chrote -c "mkdir -p ~/chrote/AgentArena_go/internal/dashboard && cp -r ~/chrote/dashboard/dist/* ~/chrote/AgentArena_go/internal/dashboard/"

# Build Go server
log "Building Go server..."
su - chrote -c "export PATH=/usr/local/go/bin:\$PATH && cd ~/chrote/AgentArena_go && go build -o ~/chrote-server ./cmd/server"

# Build vendored tools if present
log "Building vendored tools..."
su - chrote -c '
export PATH=/usr/local/go/bin:$PATH
cd ~/chrote
if [ -f vendor/gastown/go.mod ]; then
    cd vendor/gastown && go build -o ~/.local/bin/gt ./cmd/gt && cd ../..
    echo "Built gastown (gt)"
fi
if [ -f vendor/beads/go.mod ]; then
    cd vendor/beads && go build -o ~/.local/bin/bd ./cmd/bd && cd ../..
    echo "Built beads (bd)"
fi
if [ -f vendor/beads_viewer/go.mod ]; then
    cd vendor/beads_viewer && go build -o ~/.local/bin/bv ./cmd/bv && cd ../..
    echo "Built beads_viewer (bv)"
fi
'

log "Build complete"

# ============================================================================
# Phase 6: User Environment
# ============================================================================
log "Phase 6: Configuring user environment..."

cat >> /home/chrote/.bashrc << 'EOF'

# CHROTE Environment
export TMUX_TMPDIR=/run/tmux/chrote
export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"
export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8

# Default to /code directory
cd /code 2>/dev/null || cd ~
EOF

chown chrote:chrote /home/chrote/.bashrc

log "User environment configured"

# ============================================================================
# Phase 7: Systemd Services
# ============================================================================
log "Phase 7: Installing systemd services..."

# Terminal launch script
cat > /usr/local/bin/terminal-launch.sh << 'EOF'
#!/bin/bash
export TMUX_TMPDIR=/run/tmux/chrote
export LANG=en_US.UTF-8
cd /code

SESSION="$1"
if [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    exec tmux attach-session -t "$SESSION"
else
    exec bash -l
fi
EOF
chmod +x /usr/local/bin/terminal-launch.sh

# chrote-server.service
cat > /etc/systemd/system/chrote-server.service << 'EOF'
[Unit]
Description=CHROTE Server (Go)
After=network.target

[Service]
Type=simple
User=chrote
Group=chrote
WorkingDirectory=/home/chrote/chrote
Environment=TMUX_TMPDIR=/run/tmux/chrote
Environment=PORT=8080
ExecStart=/home/chrote/chrote-server --start-ttyd=false
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# chrote-ttyd.service
cat > /etc/systemd/system/chrote-ttyd.service << 'EOF'
[Unit]
Description=CHROTE Web Terminal (ttyd)
After=network.target

[Service]
Type=simple
User=chrote
Group=chrote
Environment=TMUX_TMPDIR=/run/tmux/chrote
Environment=LANG=en_US.UTF-8
ExecStart=/usr/local/bin/ttyd -p 7681 -W -a /usr/local/bin/terminal-launch.sh
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Enable and start services
systemctl daemon-reload
systemctl enable chrote-server chrote-ttyd
systemctl start chrote-server chrote-ttyd || warn "Services may need WSL restart to start properly"

log "Systemd services installed"

# ============================================================================
# Phase 8: Tailscale (Optional)
# ============================================================================
log "Phase 8: Tailscale setup..."

if ! command -v tailscale &>/dev/null; then
    log "Installing Tailscale..."
    curl -fsSL https://tailscale.com/install.sh | sh
    echo ""
    warn "Tailscale installed. To connect, run:"
    warn "  sudo tailscale up --authkey=YOUR_KEY --hostname=chrote"
else
    log "Tailscale already installed"
    if tailscale status &>/dev/null; then
        log "Tailscale is connected"
    else
        warn "Tailscale installed but not connected. Run:"
        warn "  sudo tailscale up --authkey=YOUR_KEY --hostname=chrote"
    fi
fi

# ============================================================================
# Validation
# ============================================================================
log ""
log "============================================"
log "  CHROTE WSL Setup Complete!"
log "============================================"
log ""
log "Next steps:"
log "  1. Restart WSL: wsl --shutdown"
log "  2. Start WSL:   wsl -d Ubuntu"
log "  3. Check services: systemctl status chrote-server chrote-ttyd"
log "  4. Test API: curl http://localhost:8080/api/health"
log "  5. Open browser: http://chrote:8080 (after Tailscale)"
log ""
log "Troubleshooting:"
log "  - Check logs: journalctl -u chrote-server -f"
log "  - Verify tmux: su - chrote -c 'tmux list-sessions'"
log ""
