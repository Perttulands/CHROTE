# CHROTE Operator Workflows

Day-to-day operational procedures with copy-pasteable commands.

---

## Table of Contents

1. [Starting CHROTE](#starting-chrote)
2. [Stopping CHROTE](#stopping-chrote)
3. [Service Management](#service-management)
4. [Session Operations](#session-operations)
5. [Gastown Workflows](#gastown-workflows)
6. [File Operations](#file-operations)
7. [Troubleshooting](#troubleshooting)
8. [Maintenance](#maintenance)

---

## Starting CHROTE

### Quick Start (Windows PowerShell)

```powershell
# Start CHROTE and open browser
.\Chrote-Toggle.ps1

# Start with status check
.\Chrote-Toggle.ps1 -Status
```

### Manual Start (WSL)

```bash
# Start both services
sudo systemctl start chrote-server chrote-ttyd

# Verify services are running
systemctl status chrote-server chrote-ttyd
```

### First-Time Setup

```powershell
# Run from CHROTE directory in PowerShell
.\Chrote-Toggle.ps1 -Setup
```

---

## Stopping CHROTE

### Clean Shutdown (PowerShell)

```powershell
# Stop all CHROTE services
.\Chrote-Toggle.ps1 -Stop
```

### Manual Stop (WSL)

```bash
# Stop services gracefully
sudo systemctl stop chrote-server chrote-ttyd

# Full WSL shutdown (stops everything)
wsl --shutdown
```

### Emergency Stop

```bash
# Kill all tmux sessions immediately
tmux kill-server

# Force restart services
sudo systemctl restart chrote-server chrote-ttyd
```

---

## Service Management

### Check Service Status

```bash
# Both services at once
systemctl status chrote-server chrote-ttyd

# Individual service details
journalctl -u chrote-server -n 50

# Follow logs in real-time
journalctl -u chrote-server -f
```

### Restart Services

```bash
# Restart server only (after code changes)
sudo systemctl restart chrote-server

# Restart terminal proxy (if terminals freeze)
sudo systemctl restart chrote-ttyd

# Restart both
sudo systemctl restart chrote-server chrote-ttyd
```

### View Logs (PowerShell)

```powershell
# Watch logs from Windows
.\Chrote-Toggle.ps1 -Logs
```

### View Logs (WSL)

```bash
# Server logs
journalctl -u chrote-server -f

# Terminal logs
journalctl -u chrote-ttyd -f

# Combined logs
journalctl -u chrote-server -u chrote-ttyd -f

# Logs since last boot
journalctl -u chrote-server -b
```

---

## Session Operations

### List Sessions

```bash
# List all tmux sessions
tmux list-sessions

# List with format
tmux list-sessions -F "#{session_name}: #{session_windows} windows"
```

### Create Sessions

```bash
# Create named session
tmux new-session -d -s shell1

# Create with specific starting directory
tmux new-session -d -s myproject -c /code/myproject

# Create and attach immediately
tmux new-session -s interactive
```

### Attach to Sessions

```bash
# Attach to named session
tmux attach -t shell1

# Attach or create if doesn't exist
tmux new-session -A -s shell1
```

### Kill Sessions

```bash
# Kill specific session
tmux kill-session -t shell1

# Kill all sessions (nuclear option)
tmux kill-server

# Kill sessions matching pattern
tmux list-sessions -F "#{session_name}" | grep "^gt-" | xargs -I {} tmux kill-session -t {}
```

### Rename Sessions

```bash
# Rename session
tmux rename-session -t oldname newname
```

---

## Gastown Workflows

### Starting Gastown

```bash
# Enter WSL first
wsl

# Verify tools are installed
which gt bd bv

# Start the orchestrator
gt start gastown

# Check status
gt status
```

### Check Hook for Work

```bash
# Check if work is assigned
gt hook

# Check inbox for messages
gt mail inbox
```

### Polecat Operations

```bash
# View current hook status
gt hook

# Claim work from hook
gt hook claim

# Complete work
gt hook complete

# Report issue
gt mail send --human -s "Help needed" -m "Description of problem"
```

### Mayor Communication

```bash
# Attach to Mayor session
gt mayor attach

# Send message to Mayor
gt mail send mayor/ -s "Task request" -m "Please do X"

# Check Mayor's response
gt mail inbox
```

### Beads (Issue Tracking)

```bash
# Onboard to a project
bd onboard

# Find available work
bd ready

# View issue details
bd show <issue-id>

# Claim work
bd update <issue-id> --status in_progress

# Complete work
bd close <issue-id>

# Sync with git
bd sync
```

### Witness Escalation

```bash
# Escalate to witness
gt mail send <rig>/witness -s "HELP" -m "Worker stuck on X"

# Check witness status
gt status
```

### Session Monitoring

```bash
# Peek at what agents are doing
gt peek

# View specific rig
gt peek <rig-name>
```

---

## File Operations

### Navigate Filesystem

```bash
# Working directory
cd /code

# Project files
ls -la /code

# Vault (read-only storage)
ls -la /vault
```

### Upload Files (via Dashboard)

1. Open Files tab in dashboard
2. Navigate to target directory
3. Drag files to upload zone
4. Optionally add a note (creates `.note` sidecar file)

### Send Files to Agents (Inbox)

1. Open Files tab
2. Navigate to Inbox panel
3. Drag files to drop zone
4. Add note describing the files
5. Click Send

Files land in `/code/incoming/`

### Access from Windows

```powershell
# Open in Explorer
explorer.exe \\wsl$\Ubuntu-24.04\home\chrote\chrote

# Open in VS Code
code --remote wsl+Ubuntu-24.04 /home/chrote/chrote
```

---

## Troubleshooting

### Services Won't Start

```bash
# Check service status and errors
systemctl status chrote-server --no-pager -l
systemctl status chrote-ttyd --no-pager -l

# View recent errors
journalctl -u chrote-server -p err -n 20

# Check if ports are in use
ss -tlnp | grep -E "8080|7681"
```

### Terminal Shows Black Screen

```bash
# Restart terminal service
sudo systemctl restart chrote-ttyd

# Check if tmux socket exists
ls -la /run/tmux/chrote/

# Verify TMUX_TMPDIR
echo $TMUX_TMPDIR
# Should be: /run/tmux/chrote
```

### Sessions Disappear

```bash
# Check tmux socket
ls -la /run/tmux/chrote/

# Verify socket permissions
stat /run/tmux/chrote/default

# Check if tmux server is running
pgrep -la tmux
```

### API Not Responding

```bash
# Test health endpoint
curl http://localhost:8080/api/health

# Check server process
pgrep -la chrote-server

# Restart server
sudo systemctl restart chrote-server
```

### Full Reset

```powershell
# From PowerShell - complete reset
wsl --shutdown
Start-Sleep -Seconds 5
.\Chrote-Toggle.ps1
```

```bash
# From WSL - service reset
sudo systemctl stop chrote-server chrote-ttyd
tmux kill-server
sudo systemctl start chrote-server chrote-ttyd
```

### Permission Errors

```bash
# Fix ownership of code directory
sudo chown -R chrote:chrote /home/chrote/chrote

# Fix tmux socket directory
sudo mkdir -p /run/tmux/chrote
sudo chown chrote:chrote /run/tmux/chrote
chmod 700 /run/tmux/chrote
```

---

## Maintenance

### Update CHROTE

```bash
# Pull latest code
cd /code
git pull

# Rebuild dashboard
cd dashboard
npm install
npm run build
cp -r dist ../src/internal/dashboard/

# Rebuild server
cd ../src
go build -o ../chrote-server ./cmd/server

# Restart services
sudo systemctl restart chrote-server
```

### Clean Up Sessions

```bash
# Kill all gastown sessions
tmux list-sessions -F "#{session_name}" | grep "^gt-" | xargs -I {} tmux kill-session -t {}

# Kill all HQ sessions
tmux list-sessions -F "#{session_name}" | grep "^hq-" | xargs -I {} tmux kill-session -t {}

# Keep only named sessions (shell, main)
tmux list-sessions -F "#{session_name}" | grep -v -E "^(shell|main)" | xargs -I {} tmux kill-session -t {}
```

### Disk Cleanup

```bash
# Clean npm cache
npm cache clean --force

# Clean Go cache
go clean -cache

# Remove old logs
sudo journalctl --vacuum-time=7d

# Clean test artifacts
rm -rf /code/dashboard/playwright-report
rm -rf /code/dashboard/test-results
```

### Backup Configuration

```bash
# Backup systemd services
sudo cp /etc/systemd/system/chrote-*.service ~/chrote-backup/

# Backup any local config
cp -r ~/.config/chrote ~/chrote-backup/
```

### Check System Health

```bash
# Disk usage
df -h /code /home/chrote

# Memory usage
free -h

# Running processes
ps aux | grep -E "chrote|tmux|ttyd"

# Service health
systemctl is-active chrote-server chrote-ttyd
```

---

## Quick Reference Card

| Task | Command |
|------|---------|
| Start CHROTE | `.\Chrote-Toggle.ps1` |
| Stop CHROTE | `.\Chrote-Toggle.ps1 -Stop` |
| Check status | `.\Chrote-Toggle.ps1 -Status` |
| View logs | `journalctl -u chrote-server -f` |
| List sessions | `tmux list-sessions` |
| Kill all sessions | `tmux kill-server` |
| Restart services | `sudo systemctl restart chrote-server chrote-ttyd` |
| Check hook | `gt hook` |
| Check inbox | `gt mail inbox` |
| Find work | `bd ready` |
| Full reset | `wsl --shutdown && .\Chrote-Toggle.ps1` |

---

## Access Points

| Service | URL | Purpose |
|---------|-----|---------|
| Dashboard (local) | `http://localhost:8080` | Main UI |
| Dashboard (Tailscale) | `http://chrote:8080` | Remote access |
| Terminal proxy | `http://localhost:8080/terminal/` | Raw ttyd |
| Health check | `http://localhost:8080/api/health` | API status |
