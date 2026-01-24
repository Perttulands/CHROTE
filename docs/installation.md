# CHROTE Installation Guide

This guide covers installing CHROTE from scratch on a Windows machine with WSL2.

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Windows | 11 (or 10 with WSL2) | Must support WSL2 |
| WSL2 | Latest | `wsl --update` to ensure latest |
| Ubuntu | 24.04 LTS | Other versions may work but are untested |
| RAM | 8GB+ recommended | Agents can be memory-hungry |
| Disk | 10GB+ free | For WSL, tools, and workspace |

Optional but recommended:
- **Tailscale account** - For secure remote access from any device
- **Anthropic API key** - For Claude Code (the agents)

## Quick Install (2 Commands)

```powershell
# 1. Install Ubuntu in WSL (skip if you already have it)
wsl --install -d Ubuntu-24.04

# 2. Clone CHROTE and run setup
git clone https://github.com/YourUsername/CHROTE.git
cd CHROTE
.\Chrote-Toggle.ps1 -Setup
```

That's it. The setup script handles everything else.

## What Gets Installed

The setup script (`wsl/setup-wsl.sh`) installs and configures:

### System Configuration
- **WSL config** (`/etc/wsl.conf`) - Enables systemd, sets default user to `chrote`
- **chrote user** - Non-root user for running agents (no sudo access for security)
- **Directories** - `/run/tmux/chrote` for tmux sockets, `/code` symlink to workspace

### Dependencies
| Package | Purpose |
|---------|---------|
| curl, git, wget | Downloading tools and repos |
| tmux | Terminal multiplexer (the core of CHROTE) |
| build-essential | Compiling Go binaries |
| python3, python3-pip | For scripts and tools |
| jq | JSON processing |
| rsync | File synchronization |
| locales | UTF-8 support |

### Development Tools
| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23.4 | Building the server and Gastown tools |
| Node.js | 20.x | Building the React dashboard |
| ttyd | 1.7.7 | Web-based terminal backend |
| Claude Code | Latest | Anthropic's CLI for AI coding |

### CHROTE Components
| Component | Location | Purpose |
|-----------|----------|---------|
| chrote-server | `/home/chrote/chrote-server` | Go binary serving dashboard + API |
| Dashboard | Embedded in binary | React web UI |
| Gastown (gt) | `~/.local/bin/gt` | Agent orchestration (if vendored) |
| Beads (bd) | `~/.local/bin/bd` | Issue tracking (if vendored) |
| Beads Viewer (bv) | `~/.local/bin/bv` | Beads visualization (if vendored) |

### systemd Services
| Service | Port | Purpose |
|---------|------|---------|
| chrote-server | 8080 | Main dashboard and API server |
| chrote-ttyd | 7681 | Web terminal backend |

Both services auto-start on WSL boot.

## Post-Installation

### Verify Installation

```powershell
# Check status from Windows
.\Chrote-Toggle.ps1 -Status

# Expected output:
# WSL: Running
# chrote-server: active (running)
# chrote-ttyd: active (running)
# API: OK
```

Or from inside WSL:
```bash
systemctl status chrote-server chrote-ttyd
curl http://localhost:8080/api/health
```

### Access the Dashboard

| URL | When to Use |
|-----|-------------|
| `http://localhost:8080` | Local access on the same machine |
| `http://chrote:8080` | Remote access via Tailscale |

### Configure Tailscale (Recommended)

For remote access from phones, tablets, or other computers:

```bash
# Inside WSL
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --hostname chrote
```

Then access from any device on your Tailnet: `http://chrote:8080`

## Directory Structure

After installation, you'll have:

```
/home/chrote/
├── chrote/                 # CHROTE source code (aka /code)
│   ├── src/               # Go backend
│   ├── dashboard/         # React frontend source
│   ├── vendor/            # Gastown, Beads (if vendored)
│   └── wsl/               # Setup scripts
├── chrote-server          # Compiled Go binary
└── .local/bin/            # gt, bd, bv tools

/code → /home/chrote/chrote   (symlink)
/vault → /mnt/e/Vault         (symlink, optional)
```

## Environment Variables

Set automatically in `/home/chrote/.bashrc`:

```bash
export TMUX_TMPDIR=/run/tmux/chrote    # Dedicated tmux socket directory
export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"
export LANG=en_US.UTF-8
```

## Manual Installation (Alternative)

If the automated setup fails, you can run steps manually:

```bash
# 1. Enter WSL as root
wsl -d Ubuntu-24.04 -u root

# 2. Create chrote user
useradd -m -s /bin/bash chrote
passwd -l chrote

# 3. Install dependencies
apt-get update
apt-get install -y curl git tmux python3 jq build-essential

# 4. Install Go 1.23
curl -sLO https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz

# 5. Install Node.js
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

# 6. Install ttyd
curl -sL https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 -o /usr/local/bin/ttyd
chmod +x /usr/local/bin/ttyd

# 7. Copy CHROTE files
mkdir -p /home/chrote/chrote
cp -r /mnt/c/path/to/CHROTE/* /home/chrote/chrote/
chown -R chrote:chrote /home/chrote/chrote

# 8. Build dashboard and server
su - chrote
cd ~/chrote/dashboard && npm ci && npm run build
cp -r dist/* ../src/internal/dashboard/
cd ../src && /usr/local/go/bin/go build -o ~/chrote-server ./cmd/server

# 9. Create systemd services (see setup-wsl.sh for service files)
# 10. Enable and start services
sudo systemctl enable chrote-server chrote-ttyd
sudo systemctl start chrote-server chrote-ttyd
```

## Updating CHROTE

### Update the Dashboard and Server

```bash
# Inside WSL as chrote user
cd /code

# Pull latest changes
git pull

# Rebuild dashboard
cd dashboard && npm ci && npm run build
cp -r dist/* ../src/internal/dashboard/

# Rebuild server
cd ../src && go build -o ~/chrote-server ./cmd/server

# Restart service
sudo systemctl restart chrote-server
```

### Update Claude Code

```bash
sudo npm update -g @anthropic-ai/claude-code
claude --version
```

### Update Gastown Tools

Download latest releases from:
- gt: https://github.com/steveyegge/gastown/releases
- bd: https://github.com/beads-ai/beads/releases
- bv: https://github.com/beads-ai/beads-viewer/releases

```bash
# Example: Update gt
cd /tmp
curl -LO https://github.com/steveyegge/gastown/releases/download/vX.Y.Z/gt_X.Y.Z_linux_amd64.tar.gz
tar -xzf gt_X.Y.Z_linux_amd64.tar.gz
cp gt ~/.local/bin/gt
gt --version
```

## Uninstalling

### Remove CHROTE (Keep WSL)

```bash
# Inside WSL as root
systemctl stop chrote-server chrote-ttyd
systemctl disable chrote-server chrote-ttyd
rm /etc/systemd/system/chrote-*.service
rm -rf /home/chrote/chrote /home/chrote/chrote-server
userdel -r chrote
```

### Remove WSL Entirely

```powershell
wsl --unregister Ubuntu-24.04
```

## Troubleshooting Installation

See [troubleshooting.md](troubleshooting.md) for common issues.

### Quick Fixes

**Setup script fails with permission error:**
```powershell
# Ensure running setup with -Setup flag
.\Chrote-Toggle.ps1 -Setup
```

**WSL not found:**
```powershell
# Enable WSL and install Ubuntu
wsl --install -d Ubuntu-24.04
# Restart computer, then run setup again
```

**Services won't start:**
```bash
# Check if systemd is enabled
cat /etc/wsl.conf  # Should show [boot] systemd = true

# If not, WSL needs restart after setup
wsl --shutdown
# Then start WSL again
```

**Build fails:**
```bash
# Check Go version
/usr/local/go/bin/go version  # Should be 1.23+

# Check Node version
node --version  # Should be 20+

# Clear npm cache and retry
cd /code/dashboard
rm -rf node_modules
npm ci
```
