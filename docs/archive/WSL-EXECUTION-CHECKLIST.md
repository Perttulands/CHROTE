# WSL Migration Execution Checklist

**Purpose:** Step-by-step checklist to execute the WSL migration.
**Reference:** See [WSL-migration-plan.md](WSL-migration-plan.md) for detailed instructions.

---

## Pre-Migration

- [x] Backup current `.env` file
- [x] Note your Tailscale auth key
- [x] Ensure WSL version is 0.67.6+ (`wsl --version`)
- [x] Close VS Code and any processes using AgentArena files

---

## Phase 1: Create WSL Instance

```powershell
wsl --install -d Ubuntu-24.04
```

- [x] Ubuntu 24.04 installed
- [x] Initial user created (temporary, will add chrote later)

---

## Phase 2: Configure WSL (as root)

```bash
wsl -d Ubuntu-24.04 -u root
```

- [x] Create `/etc/wsl.conf`:
  ```ini
  [boot]
  systemd = true

  [user]
  default = chrote

  [automount]
  enabled = true
  options = "metadata,umask=22,fmask=11"
  ```
- [x] Restart WSL: `wsl --shutdown` then re-enter

---

## Phase 3: Create User (as root)

- [x] Create chrote user: `useradd -m -s /bin/bash chrote`
- [x] Lock password: `passwd -l chrote`
- [x] Create tmux socket directory:
  ```bash
  cat > /etc/tmpfiles.d/chrote-tmux.conf << 'EOF'
  d /run/tmux 0755 root root -
  d /run/tmux/chrote 0700 chrote chrote -
  EOF
  mkdir -p /run/tmux/chrote
  chown chrote:chrote /run/tmux/chrote
  chmod 0700 /run/tmux/chrote
  ```
- [x] Create /vault symlink: `ln -s /mnt/e/Vault /vault`

---

## Phase 4: Install Dependencies (as root)

```bash
apt-get update && apt-get install -y \
    curl git tmux python3 python3-pip \
    openssh-server locales build-essential

locale-gen en_US.UTF-8
```

- [x] Go installed:
  ```bash
  curl -LO https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
  rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
  rm go1.22.0.linux-amd64.tar.gz
  ```
- [x] Node.js 20 installed:
  ```bash
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
  apt-get install -y nodejs
  ```
- [x] ttyd installed:
  ```bash
  curl -L https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 \
      -o /usr/local/bin/ttyd
  chmod +x /usr/local/bin/ttyd
  ```
- [x] Claude Code installed: `npm install -g @anthropic-ai/claude-code`

---

## Phase 5: Clone and Build (as chrote)

```bash
su - chrote
```

- [x] Clone repo: `git clone https://github.com/Perttulands/CHROTE.git ~/chrote`
- [x] Create /code symlink (exit to root):
  ```bash
  exit
  ln -s /home/chrote/chrote /code
  chown -h chrote:chrote /code
  su - chrote
  ```
- [x] Build dashboard:
  ```bash
  cd ~/chrote/dashboard
  npm ci
  npm run build
  cp -r dist/ ~/chrote/src/internal/dashboard/
  ```
- [x] Build Go server:
  ```bash
  cd ~/chrote/src
  /usr/local/go/bin/go build -o ~/chrote-server ./cmd/server
  ```
- [x] Build vendored tools (if present):
  ```bash
  cd ~/chrote/vendor/gastown && /usr/local/go/bin/go build -o ~/.local/bin/gt ./cmd/gt
  cd ~/chrote/vendor/beads && /usr/local/go/bin/go build -o ~/.local/bin/bd ./cmd/bd
  cd ~/chrote/vendor/beads_viewer && /usr/local/go/bin/go build -o ~/.local/bin/bv ./cmd/bv
  ```

---

## Phase 6: User Environment

- [x] Add to `/home/chrote/.bashrc`:
  ```bash
  export TMUX_TMPDIR=/run/tmux/chrote
  export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"
  export LANG=en_US.UTF-8
  export LC_ALL=en_US.UTF-8
  cd /code 2>/dev/null || cd ~
  ```

---

## Phase 7: Systemd Services (as root)

- [x] Create `/etc/systemd/system/chrote-server.service`:
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

- [x] Create `/etc/systemd/system/chrote-ttyd.service`:
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

- [x] Create `/usr/local/bin/terminal-launch.sh`:
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

- [x] Enable services:
  ```bash
  systemctl daemon-reload
  systemctl enable chrote-server chrote-ttyd
  systemctl start chrote-server chrote-ttyd
  ```

---

## Phase 8: Tailscale (as root)

```bash
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --authkey=YOUR_KEY --hostname=chrote
```

- [x] Tailscale installed and connected
- [x] Hostname is `chrote`

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
```

- [x] Services running
- [x] API responds
- [x] Tailscale connected
- [x] Dashboard loads at http://chrote:8080
- [x] Can create tmux session
- [x] Can attach to session in terminal
- [x] File browser works

---

## Post-Migration

- [x] Update Windows shortcut (`Chrote-Toggle.ps1`)
- [x] Test from another device via Tailscale
- [x] Consider removing Docker containers: `docker compose down`
- [x] Rename `AgentArena_go` → `src` (close VS Code first)

---

## Status: COMPLETED

Migration successfully completed January 2025.
