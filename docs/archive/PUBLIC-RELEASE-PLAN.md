# CHROTE: Public Release Plan

**Status:** Future milestone (after WSL migration is stable)
**Goal:** Transform CHROTE from a personal tool to a public GitHub project anyone can install

---

## Current State vs Public Release

| Aspect | Current (Personal) | Public Release |
|--------|-------------------|----------------|
| Paths | Hardcoded `/code`, `/vault` | Configurable via env/flags |
| User | Hardcoded `chrote` | Uses `$USER` |
| Systemd | Root (`/etc/systemd/system/`) | User mode (`~/.config/systemd/user/`) |
| Binaries | Manual build | GitHub Actions → Releases |
| Installation | 10-step migration guide | `install.sh` one-liner |

---

## Phase 1: Code Generalization

### 1.1 Make `allowedRoots` Configurable

**Current** (`files.go`):
```go
allowedRoots: []string{"/code", "/vault"}
```

**Change to:**
```go
// From environment or flag
allowedRoots := strings.Split(os.Getenv("CHROTE_ALLOWED_ROOTS"), ",")
if len(allowedRoots) == 0 || allowedRoots[0] == "" {
    allowedRoots = []string{"/code", "/vault"} // defaults
}
```

### 1.2 Make Paths Configurable

Add to `main.go`:
```go
var (
    port         = flag.Int("port", 8080, "HTTP server port")
    allowedRoots = flag.String("allowed-roots", "/code,/vault", "Comma-separated allowed file roots")
    tmuxSocket   = flag.String("tmux-socket", "", "tmux socket path (default: $TMUX_TMPDIR or /tmp)")
)
```

### 1.3 Config File Support

Create `~/.chrote/config.yaml`:
```yaml
# CHROTE Configuration
server:
  port: 8080

filesystem:
  allowed_roots:
    - ~/projects        # User's workspace
    - ~/reference       # Read-only reference materials

tailscale:
  hostname: chrote      # Optional, for Tailscale users
```

---

## Phase 2: Installation Script

### 2.1 `install.sh` Structure

```bash
#!/bin/bash
set -e

REPO="Perttulands/CHROTE"
INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.chrote"
SERVICE_DIR="$HOME/.config/systemd/user"

echo "🚀 Installing CHROTE..."

# 1. Detect environment
detect_wsl() {
    if grep -qi microsoft /proc/version 2>/dev/null; then
        echo "wsl"
    else
        echo "linux"
    fi
}

# 2. Install dependencies
install_deps() {
    if command -v apt-get &>/dev/null; then
        sudo apt-get update
        sudo apt-get install -y tmux curl
    elif command -v brew &>/dev/null; then
        brew install tmux curl
    fi
}

# 3. Download binary
download_binary() {
    LATEST=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep tag_name | cut -d'"' -f4)
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
    esac

    curl -sL "https://github.com/$REPO/releases/download/$LATEST/chrote-server-linux-$ARCH" \
        -o "$INSTALL_DIR/chrote-server"
    chmod +x "$INSTALL_DIR/chrote-server"
}

# 4. Setup workspace
setup_workspace() {
    read -p "Workspace directory [$HOME/chrote-workspace]: " WORKSPACE
    WORKSPACE="${WORKSPACE:-$HOME/chrote-workspace}"
    mkdir -p "$WORKSPACE"

    # Create config
    mkdir -p "$CONFIG_DIR"
    cat > "$CONFIG_DIR/config.yaml" << EOF
server:
  port: 8080
filesystem:
  allowed_roots:
    - $WORKSPACE
EOF
}

# 5. Setup systemd (user mode)
setup_systemd() {
    mkdir -p "$SERVICE_DIR"
    cat > "$SERVICE_DIR/chrote.service" << EOF
[Unit]
Description=CHROTE Server
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/chrote-server --config $CONFIG_DIR/config.yaml
Restart=on-failure
Environment=TMUX_TMPDIR=%t/tmux

[Install]
WantedBy=default.target
EOF

    systemctl --user daemon-reload
    systemctl --user enable chrote
}

# 6. Install ttyd
install_ttyd() {
    TTYD_VERSION="1.7.7"
    curl -sL "https://github.com/tsl0922/ttyd/releases/download/$TTYD_VERSION/ttyd.$(uname -m)" \
        -o "$INSTALL_DIR/ttyd"
    chmod +x "$INSTALL_DIR/ttyd"
}

# Run installation
install_deps
download_binary
install_ttyd
setup_workspace
setup_systemd

echo ""
echo "✅ CHROTE installed!"
echo ""
echo "Start:   systemctl --user start chrote"
echo "Stop:    systemctl --user stop chrote"
echo "Status:  systemctl --user status chrote"
echo "Logs:    journalctl --user -u chrote -f"
echo ""
echo "Dashboard: http://localhost:8080"
```

### 2.2 Uninstall Script

```bash
#!/bin/bash
systemctl --user stop chrote 2>/dev/null
systemctl --user disable chrote 2>/dev/null
rm -f ~/.config/systemd/user/chrote.service
rm -f ~/.local/bin/chrote-server
rm -f ~/.local/bin/ttyd
rm -rf ~/.chrote
echo "CHROTE uninstalled"
```

---

## Phase 3: GitHub Actions CI/CD

### 3.1 Release Workflow (`.github/workflows/release.yml`)

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      # Build dashboard
      - name: Build Dashboard
        run: |
          cd dashboard
          npm ci
          npm run build
          cp -r dist ../src/internal/dashboard/

      # Build Go binaries
      - name: Build Binaries
        run: |
          cd src
          GOOS=linux GOARCH=amd64 go build -o ../dist/chrote-server-linux-amd64 ./cmd/server
          GOOS=linux GOARCH=arm64 go build -o ../dist/chrote-server-linux-arm64 ./cmd/server

      # Create release
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/chrote-server-linux-amd64
            dist/chrote-server-linux-arm64
            install.sh
```

---

## Phase 4: Documentation for Public

### 4.1 README.md Structure

```markdown
# CHROTE

> Claude Has Root Over This Environment

A web dashboard for supervising many AI coding agents running in tmux sessions.

## Quick Install (Linux/WSL)

```bash
curl -sL https://raw.githubusercontent.com/Perttulands/CHROTE/main/install.sh | bash
```

## Features

- 🖥️ Watch 1-4 terminal sessions simultaneously
- 📁 File browser with drag-drop uploads
- 🎯 Beads integration for issue tracking
- 🎵 Built-in music player (because why not)

## Requirements

- Linux or WSL2
- tmux
- Modern browser

## Manual Installation

[See INSTALL.md for detailed instructions]

## Configuration

[See CONFIG.md for all options]

## For Gastown Users

CHROTE is designed to work with [Gastown](https://github.com/steveyegge/gastown).
[See GASTOWN.md for integration details]
```

---

## Checklist for Public Release

### Code Changes
- [ ] Make `allowedRoots` configurable via env/flag
- [ ] Add `--config` flag for YAML config file
- [ ] Remove hardcoded `chrote` user references
- [ ] Make `TMUX_TMPDIR` default to user-friendly path
- [ ] Add version flag (`--version`)

### Build/Release
- [ ] Create GitHub Actions workflow for releases
- [ ] Test binary on fresh Ubuntu VM
- [ ] Test binary on fresh WSL2 install
- [ ] Add ARM64 builds for Raspberry Pi / Mac (if applicable)

### Scripts
- [ ] Write `install.sh`
- [ ] Write `uninstall.sh`
- [ ] Test on fresh WSL2 instance

### Documentation
- [ ] Rewrite README.md for public audience
- [ ] Create INSTALL.md (manual steps)
- [ ] Create CONFIG.md (all options)
- [ ] Create CONTRIBUTING.md
- [ ] Add LICENSE file

### Polish
- [ ] Create logo/icon
- [ ] Record demo GIF for README
- [ ] Write blog post / announcement

---

## Timeline

| Phase | Milestone | Dependency |
|-------|-----------|------------|
| 1 | WSL migration complete | Current work |
| 2 | Code generalization | Phase 1 |
| 3 | GitHub Actions CI | Phase 2 |
| 4 | install.sh tested | Phase 3 |
| 5 | Documentation complete | Phase 4 |
| 6 | Public release | All phases |

---

## Notes

- User-mode systemd (`~/.config/systemd/user/`) avoids need for root
- GitHub Releases provide pre-built binaries so users don't need Go/Node
- Config file is optional - sensible defaults should work out of the box
- The `/code` symlink becomes optional for users who configure their own paths
