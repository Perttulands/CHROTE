# CHROTE: WSL Migration Plan

## What Changes for You

### Before (Docker)

```
You:        Double-click "CHROTE Toggle" on desktop
              ↓
Docker:     Starts container, takes 10-30 seconds
              ↓
Browser:    http://chrote:8080 → Dashboard
              ↓
Editing:    VS Code opens E:\Docker\AgentArena (Windows)
              ↓
Agents:     Run as ROOT with IS_SANDBOX=1 hack
```

### After (WSL)

```
You:        Double-click "CHROTE Toggle" on desktop
              ↓
WSL:        Services already running (systemd auto-start)
              ↓
Browser:    http://chrote:8080 → Dashboard (same URL!)
              ↓
Editing:    VS Code "Remote - WSL" opens /home/chrote/chrote
              ↓
Agents:     Run as 'chrote' user (proper Linux permissions)
```

### User Experience Changes

| Action | Before (Docker) | After (WSL) |
|--------|-----------------|-------------|
| **Start CHROTE** | Toggle script → Docker Compose up (10-30s) | Already running (systemd) |
| **Stop CHROTE** | Toggle script → Docker Compose down | `wsl --shutdown` (or leave running) |
| **Edit code** | VS Code on Windows path | VS Code Remote-WSL (faster) |
| **Dashboard URL** | http://chrote:8080 | http://chrote:8080 (unchanged) |
| **File browser** | /code, /vault | /code, /vault (symlinks, same paths) |
| **npm install** | 2-5 minutes (slow) | 5-10 seconds (native Linux) |
| **Git operations** | Slow | Fast |
| **Rebuild** | `docker compose build` | Just restart service |

### What Stays the Same

- Dashboard UI - identical
- Tailscale access - same hostname `chrote`
- Session management - same tmux workflow
- Gastown/Beads - same commands (`gt`, `bd`, `bv`)
- File paths inside environment - `/code`, `/vault`

### What's Better

1. **No root execution** - agents run as `chrote` user, no `IS_SANDBOX=1` hack
2. **Faster filesystem** - native Linux ext4, not Windows→WSL bridge
3. **Auto-start** - systemd starts services on WSL boot
4. **Simpler debugging** - standard Linux, standard tools
5. **Proper security model** - user isolation, not container boundary

---

## Architecture

### Target Stack

```
┌─────────────────────────────────────────────────────────────┐
│                         WSL2                                 │
│                    (Ubuntu 24.04)                            │
│                                                              │
│  User: chrote (non-root, UID 1000, NO sudo)                  │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                    systemd                           │    │
│  │                                                      │    │
│  │  chrote-server.service     chrote-ttyd.service      │    │
│  │  (Go binary :8080)         (web terminal :7681)     │    │
│  │       │                           │                  │    │
│  │       │     ┌─────────────────────┘                  │    │
│  │       │     │                                        │    │
│  │       ▼     ▼                                        │    │
│  │  ┌─────────────────────────────────────────────┐    │    │
│  │  │              tmux server                     │    │    │
│  │  │    Socket: /run/tmux/chrote/default          │    │    │
│  │  └─────────────────────────────────────────────┘    │    │
│  │                      │                               │    │
│  │       ┌──────────────┼──────────────┐               │    │
│  │       ▼              ▼              ▼               │    │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────┐           │    │
│  │  │ Agent 1 │   │ Agent 2 │   │ Agent N │           │    │
│  │  │ Claude  │   │ Claude  │   │ Claude  │           │    │
│  │  └─────────┘   └─────────┘   └─────────┘           │    │
│  │                                                      │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                              │
│  Tailscale (native) → hostname: chrote                       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│  /home/chrote/chrote/     Native Linux filesystem (FAST)    │
│  /vault                   → /mnt/e/Vault (read-only)        │
└─────────────────────────────────────────────────────────────┘
```

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **API Server** | Go (not Node.js) | Single binary, embeds dashboard, no nginx needed |
| **Filesystem** | Native WSL (`/home/chrote/`) | 10-50x faster than `/mnt/e/` |
| **User** | `chrote` (no sudo) | Agents can't escalate privileges |
| **Services** | systemd | Auto-start, proper process management |
| **Web server** | Go serves static files | Eliminates nginx dependency |

### What's Eliminated

- ~~nginx~~ - Go server serves dashboard + proxies ttyd
- ~~Node.js API~~ - Go server replaces it
- ~~Docker~~ - WSL is the runtime
- ~~IS_SANDBOX=1~~ - proper user permissions instead
- ~~Root execution~~ - everything runs as `chrote`

---

## Filesystem Layout

### In WSL (Native - Fast)

```
/home/chrote/chrote/           # Git clone of this repo
├── src/                       # Go server
│   ├── cmd/server/main.go
│   └── internal/
│       ├── api/
│       ├── core/
│       ├── proxy/
│       └── dashboard/dist/    # Built React app (embedded)
├── dashboard/                 # React source
├── wsl/                       # Systemd services, scripts
└── vendor/                    # gastown, beads, beads_viewer
```

### Symlinks (Compatibility)

```
/code   → /home/chrote/chrote      (chrote:chrote)
/vault  → /mnt/e/Vault             (root:root, read-only)
```

These symlinks ensure existing scripts that reference `/code` and `/vault` continue to work.

---

## Prerequisites

### Windows Requirements

```powershell
# Check WSL version (need 0.67.6+ for systemd)
wsl --version

# If outdated:
wsl --update
```

- Windows 11 (or Windows 10 with recent updates)
- WSL version 0.67.6 or later

### What You Need

- GitHub access to clone repo
- Tailscale auth key (get from Tailscale admin console)
- ~30 minutes for initial setup

---

## Implementation

### Phase 1: Create WSL Instance

```powershell
# Install fresh Ubuntu 24.04
wsl --install -d Ubuntu-24.04

# After initial setup, open Ubuntu and become root
wsl -d Ubuntu-24.04 -u root
```

### Phase 2: Configure WSL

Create `/etc/wsl.conf`:

```ini
[boot]
systemd = true

[user]
default = chrote

[automount]
enabled = true
options = "metadata,umask=22,fmask=11"
```

Restart WSL to apply:

```powershell
wsl --shutdown
wsl -d Ubuntu-24.04 -u root
```

### Phase 3: Create User

```bash
# Create chrote user (NO sudo access)
useradd -m -s /bin/bash chrote

# Lock password (SSH key only, or no remote access)
passwd -l chrote

# Create tmux socket directory (persists across reboots)
mkdir -p /etc/tmpfiles.d
cat > /etc/tmpfiles.d/chrote-tmux.conf << 'EOF'
d /run/tmux 0755 root root -
d /run/tmux/chrote 0700 chrote chrote -
EOF

# Create it now
mkdir -p /run/tmux/chrote
chown chrote:chrote /run/tmux/chrote
chmod 0700 /run/tmux/chrote

# Create /vault symlink (read-only access to Windows vault)
ln -s /mnt/e/Vault /vault
```

### Phase 4: Install Dependencies

```bash
# System packages
apt-get update && apt-get install -y \
    curl git tmux python3 python3-pip \
    openssh-server locales build-essential

locale-gen en_US.UTF-8

# Go (from official source for latest version)
curl -LO https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
rm go1.22.0.linux-amd64.tar.gz

# Node.js 20 LTS (for Claude Code and dashboard build)
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

# ttyd (web terminal)
curl -L https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 \
    -o /usr/local/bin/ttyd
chmod +x /usr/local/bin/ttyd

# Claude Code
npm install -g @anthropic-ai/claude-code

# Create directories for chrote user
mkdir -p /home/chrote/.local/bin
chown -R chrote:chrote /home/chrote/.local
```

### Phase 5: Clone and Build

```bash
# Switch to chrote user
su - chrote

# Clone repo
git clone https://github.com/Perttulands/CHROTE.git ~/chrote

# Create /code symlink (exit to root, then return)
exit
ln -s /home/chrote/chrote /code
chown -h chrote:chrote /code
su - chrote

# Build dashboard
cd ~/chrote/dashboard
npm ci
npm run build

# Copy built dashboard to Go server
cp -r dist/ ~/chrote/src/internal/dashboard/

# Build Go server
cd ~/chrote/src
/usr/local/go/bin/go build -o ~/chrote-server ./cmd/server

# Build vendored tools (if present)
cd ~/chrote
if [ -f vendor/gastown/go.mod ]; then
    cd vendor/gastown && /usr/local/go/bin/go build -o ~/.local/bin/gt ./cmd/gt && cd ../..
fi
if [ -f vendor/beads/go.mod ]; then
    cd vendor/beads && /usr/local/go/bin/go build -o ~/.local/bin/bd ./cmd/bd && cd ../..
fi
if [ -f vendor/beads_viewer/go.mod ]; then
    cd vendor/beads_viewer && /usr/local/go/bin/go build -o ~/.local/bin/bv ./cmd/bv && cd ../..
fi
```

### Phase 6: User Environment

Add to `/home/chrote/.bashrc`:

```bash
# Tmux socket location
export TMUX_TMPDIR=/run/tmux/chrote

# Go
export PATH="/usr/local/go/bin:$PATH"

# Vendored tools
export PATH="$HOME/.local/bin:$PATH"

# Locale
export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8

# Default directory
cd /code 2>/dev/null || cd ~
```

### Phase 7: Systemd Services

As root, create `/etc/systemd/system/chrote-server.service`:

```ini
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
ExecStart=/home/chrote/chrote-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/chrote-ttyd.service`:

```ini
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
```

Create `/usr/local/bin/terminal-launch.sh`:

```bash
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
```

```bash
chmod +x /usr/local/bin/terminal-launch.sh
```

Enable services:

```bash
systemctl daemon-reload
systemctl enable chrote-server chrote-ttyd
systemctl start chrote-server chrote-ttyd
```

### Phase 8: Tailscale

```bash
# As root
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --authkey=YOUR_KEY --hostname=chrote
```

---

## Validation

```bash
# Check services
systemctl status chrote-server chrote-ttyd

# Test API
curl http://localhost:8080/api/health

# Test Tailscale
tailscale status
curl http://chrote:8080/api/health

# List tmux sessions
su - chrote -c "TMUX_TMPDIR=/run/tmux/chrote tmux list-sessions"
```

Open browser: http://chrote:8080

- [ ] Dashboard loads
- [ ] Can create tmux session
- [ ] Can attach to session in terminal
- [ ] File browser works
- [ ] Sessions visible in panel

---

## Windows Launcher

Update `Chrote-Toggle.ps1` to manage WSL instead of Docker:

```powershell
# Start WSL (services auto-start via systemd)
wsl -d Ubuntu-24.04 echo "CHROTE started"

# Open browser
Start-Process "http://chrote:8080"

# To stop everything
# wsl --shutdown
```

---

## Troubleshooting

### systemd not working

```bash
# Check WSL version
wsl --version  # Need 0.67.6+

# Verify wsl.conf
cat /etc/wsl.conf  # Must have [boot] systemd = true

# Restart WSL
wsl --shutdown
```

### tmux sessions not visible

```bash
# All processes must use same TMUX_TMPDIR
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote

# Check socket exists
ls -la /run/tmux/chrote/
```

### Permission denied

```bash
# Verify chrote owns the code directory
ls -la /home/chrote/chrote

# Verify symlinks
ls -la /code /vault
```

### Go server won't start

```bash
# Check logs
journalctl -u chrote-server -f

# Test manually
su - chrote -c "cd /home/chrote/chrote && ./chrote-server"
```

---

## Summary

| Aspect | Docker (Old) | WSL (New) |
|--------|--------------|-----------|
| Runtime | Container | Native Linux |
| User | root + IS_SANDBOX=1 | chrote (non-root) |
| Server | Node.js + nginx | Go (single binary) |
| Filesystem | /mnt/e/ (slow) | /home/chrote/ (fast) |
| Services | docker compose | systemd |
| Start time | 10-30 seconds | Already running |
| Security | Container boundary | User permissions |
