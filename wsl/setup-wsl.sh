#!/bin/bash
set -e

RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
NC="\033[0m"

log() { echo -e "${GREEN}[CHROTE]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

[ "$EUID" -ne 0 ] && error "Must run as root"

log "Starting CHROTE WSL setup..."

# Phase 1: WSL config
log "Phase 1: System configuration..."
if ! grep -q "systemd = true" /etc/wsl.conf 2>/dev/null; then
    cat > /etc/wsl.conf << EOF
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
    NEED_RESTART=true
fi

# Phase 2: Create user
log "Phase 2: Creating chrote user..."
if ! id chrote &>/dev/null; then
    useradd -m -s /bin/bash chrote
    passwd -l chrote
fi

# Phase 3: Directories
log "Phase 3: Creating directories..."
mkdir -p /etc/tmpfiles.d /run/tmux/chrote /home/chrote/.local/bin
cat > /etc/tmpfiles.d/chrote-tmux.conf << EOF
d /run/tmux 0755 root root -
d /run/tmux/chrote 0700 chrote chrote -
EOF
chown chrote:chrote /run/tmux/chrote
chmod 0700 /run/tmux/chrote
chown -R chrote:chrote /home/chrote/.local
[ ! -L /vault ] && ln -s /mnt/e/Vault /vault 2>/dev/null || true

# Phase 4: Dependencies
log "Phase 4: Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl git tmux python3 python3-pip locales build-essential jq wget unzip rsync >/dev/null
locale-gen en_US.UTF-8 >/dev/null

# Install Go 1.23 (required by go.mod)
GO_VERSION="1.23.4"
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go$GO_VERSION"; then
    log "Installing Go $GO_VERSION..."
    curl -sLO "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "go$GO_VERSION.linux-amd64.tar.gz"
    rm "go$GO_VERSION.linux-amd64.tar.gz"
fi
export GOTOOLCHAIN=auto

if ! command -v node &>/dev/null; then
    log "Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x 2>/dev/null | bash - >/dev/null 2>&1
    apt-get install -y -qq nodejs >/dev/null
fi

if [ ! -f /usr/local/bin/ttyd ]; then
    log "Installing ttyd..."
    curl -sL https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 -o /usr/local/bin/ttyd
    chmod +x /usr/local/bin/ttyd
fi

npm install -g @anthropic-ai/claude-code 2>/dev/null || true

# Phase 5: Copy and build
log "Phase 5: Copying and building..."
CHROTE_HOME=/home/chrote/chrote
SRC="${CHROTE_SRC:-/mnt/e/Docker/CHROTE}"  # Use env var from bootstrap, fallback for manual runs
log "Source: $SRC"
if [ ! -d "$CHROTE_HOME" ]; then
    mkdir -p "$CHROTE_HOME"
    rsync -a --exclude="node_modules" --exclude=".git" "$SRC/" "$CHROTE_HOME/" 2>/dev/null || cp -r "$SRC"/* "$CHROTE_HOME/"
    chown -R chrote:chrote "$CHROTE_HOME"
fi
[ ! -L /code ] && ln -s "$CHROTE_HOME" /code && chown -h chrote:chrote /code

log "Building dashboard..."
su - chrote -c "cd ~/chrote/dashboard && npm ci --silent 2>/dev/null && npm run build --silent 2>/dev/null"
su - chrote -c "mkdir -p ~/chrote/src/internal/dashboard && cp -r ~/chrote/dashboard/dist/* ~/chrote/src/internal/dashboard/"

log "Building Go server..."
su - chrote -c "export PATH=/usr/local/go/bin:\$PATH && cd ~/chrote/src && go build -o ~/chrote-server ./cmd/server"

# Build vendored tools (gastown, beads, beads_viewer)
log "Building vendored tools..."
su - chrote -c '
export PATH=/usr/local/go/bin:$PATH
cd ~/chrote
if [ -f vendor/gastown/go.mod ]; then
    cd vendor/gastown && go build -o ~/.local/bin/gt ./cmd/gt && cd ../..
    echo "  Built gastown (gt)"
fi
if [ -f vendor/beads/go.mod ]; then
    cd vendor/beads && go build -o ~/.local/bin/bd ./cmd/bd && cd ../..
    echo "  Built beads (bd)"
fi
if [ -f vendor/beads_viewer/go.mod ]; then
    cd vendor/beads_viewer && go build -o ~/.local/bin/bv ./cmd/bv && cd ../..
    echo "  Built beads_viewer (bv)"
fi
'

# Phase 6: User environment
log "Phase 6: Configuring environment..."
if ! grep -q "CHROTE Environment" /home/chrote/.bashrc 2>/dev/null; then
    cat >> /home/chrote/.bashrc << EOF

# CHROTE Environment
export TMUX_TMPDIR=/run/tmux/chrote
export PATH="/usr/local/go/bin:\$HOME/.local/bin:\$PATH"
export LANG=en_US.UTF-8
cd /code 2>/dev/null || cd ~
EOF
    chown chrote:chrote /home/chrote/.bashrc
fi

# Phase 7: Services
log "Phase 7: Installing services..."
cat > /usr/local/bin/terminal-launch.sh << EOF
#!/bin/bash
export TMUX_TMPDIR=/run/tmux/chrote
export LANG=en_US.UTF-8
cd /code
SESSION="\$1"
[ -n "\$SESSION" ] && tmux has-session -t "\$SESSION" 2>/dev/null && exec tmux attach-session -t "\$SESSION"
exec bash -l
EOF
chmod +x /usr/local/bin/terminal-launch.sh

cat > /etc/systemd/system/chrote-server.service << EOF
[Unit]
Description=CHROTE Server
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
[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/chrote-ttyd.service << EOF
[Unit]
Description=CHROTE Web Terminal
After=network.target
[Service]
Type=simple
User=chrote
Group=chrote
Environment=TMUX_TMPDIR=/run/tmux/chrote
Environment=LANG=en_US.UTF-8
ExecStart=/usr/local/bin/ttyd -p 7681 -W -a /usr/local/bin/terminal-launch.sh
Restart=on-failure
[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable chrote-server chrote-ttyd >/dev/null 2>&1
systemctl is-system-running &>/dev/null && systemctl start chrote-server chrote-ttyd || true

echo ""
log "============================================"
log "  CHROTE Setup Complete!"
log "============================================"
echo ""
if [ "$NEED_RESTART" = true ]; then
    warn "WSL needs restart. Run from PowerShell:"
    echo "  wsl --shutdown"
    echo "  wsl -d <your-distro>"
    echo ""
fi
log "Then open: http://localhost:8080"
