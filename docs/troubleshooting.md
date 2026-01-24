# CHROTE Troubleshooting Guide

This guide covers common issues and their solutions. For AI assistants helping users, see the "For Claude" sections.

## Critical Concept: Dev Mode vs Production Mode

**This is the #1 source of confusion.** CHROTE has two completely different ways to run the dashboard:

| Mode | URL | Server | When Changes Apply |
|------|-----|--------|-------------------|
| **Production** | `localhost:8080` | Go binary (chrote-server) | After rebuild + restart |
| **Development** | `localhost:5173` | Vite dev server | Instantly (hot reload) |

### How to Tell Which Mode You're In

- **localhost:5173** = Development mode (Vite)
- **localhost:8080** = Production mode (Go binary)
- **chrote:8080** = Production mode via Tailscale

### The Common Mistake

**Problem:** User makes a frontend change, rebuilds the Go binary, restarts the server... but nothing changes.

**Cause:** They're viewing `localhost:5173` (dev mode), but they rebuilt the production binary. The Vite dev server doesn't care about the Go binary - it serves files directly from `dashboard/src/`.

**Solution:**
- If in dev mode (`:5173`): Just save the file. Vite hot-reloads automatically.
- If in prod mode (`:8080`): Rebuild AND restart:
  ```bash
  cd dashboard && npm run build
  cp -r dist/* ../src/internal/dashboard/
  cd ../src && go build -o ~/chrote-server ./cmd/server
  sudo systemctl restart chrote-server
  ```

### For Claude: Dev vs Prod Diagnostic

**ALWAYS ask first:** "Are you viewing the dashboard at localhost:5173 or localhost:8080?"

Then:
- **5173 (dev)**: Changes to `dashboard/src/*.tsx` apply immediately. No rebuild needed. Check `vite.config.ts` for proxy issues.
- **8080 (prod)**: Changes require full rebuild + restart. Check if the binary was actually rebuilt.

**Never assume "restart the server" fixes frontend issues in dev mode.**

---

## Common Issues

### Dashboard Issues

#### Dashboard shows blank page / won't load

**Check 1: Which server is running?**
```bash
# Is Vite dev server running?
ps aux | grep vite

# Is Go server running?
systemctl status chrote-server
```

**Check 2: Browser console errors**
Open browser DevTools (F12) → Console tab. Look for:
- 404 errors = routing/proxy issue
- CORS errors = wrong origin
- WebSocket errors = terminal proxy issue

**Check 3: Rebuild from scratch**
```bash
cd /code/dashboard
rm -rf node_modules dist
npm ci
npm run build
cp -r dist/* ../src/internal/dashboard/
cd ../src && go build -o ~/chrote-server ./cmd/server
sudo systemctl restart chrote-server
```

#### Styles/CSS not updating

**In dev mode (5173):** Clear browser cache (Ctrl+Shift+R) or disable cache in DevTools.

**In prod mode (8080):** The CSS is embedded in the binary. You must:
1. Run `npm run build`
2. Copy dist to Go directory
3. Rebuild Go binary
4. Restart service

#### New API route returns 404

**In dev mode:** Add the route to `dashboard/vite.config.ts` proxy section:
```typescript
server: {
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
    // Add new route here
    '/my-new-route': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
},
```

**In prod mode:** The route must exist in the Go backend. Check `src/cmd/server/main.go` and the relevant handler's `RegisterRoutes()` function.

---

### Terminal Issues

#### Terminal shows black screen

```bash
# Check ttyd service
systemctl status chrote-ttyd

# Restart if needed
sudo systemctl restart chrote-ttyd

# Check if ttyd port is listening
ss -tlnp | grep 7681
```

#### Terminal connects but no prompt

```bash
# Check tmux socket directory
ls -la /run/tmux/chrote/

# Check TMUX_TMPDIR is set
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote

# Check if tmux sessions exist
TMUX_TMPDIR=/run/tmux/chrote tmux list-sessions
```

#### WebSocket connection fails

Check browser console for WebSocket errors. Common causes:

1. **Proxy not configured** (dev mode): Check `vite.config.ts` has WebSocket proxy:
   ```typescript
   '/terminal': {
     target: 'http://localhost:7681',
     ws: true,  // This is required!
   },
   ```

2. **ttyd not running**: `sudo systemctl restart chrote-ttyd`

3. **Wrong port**: Terminal proxy should point to 7681 (ttyd), not 8080 (Go server)

---

### Session Issues

#### Sessions don't appear in sidebar

```bash
# Check tmux socket
ls -la /run/tmux/chrote/

# List sessions with correct socket
TMUX_TMPDIR=/run/tmux/chrote tmux list-sessions

# If socket missing, create it
sudo mkdir -p /run/tmux/chrote
sudo chown chrote:chrote /run/tmux/chrote
sudo chmod 700 /run/tmux/chrote
```

#### Sessions disappear after WSL restart

The `/run/tmux/chrote` directory is in tmpfs and gets cleared on restart. This is expected.

To persist the directory across restarts, the setup creates `/etc/tmpfiles.d/chrote-tmux.conf`. Verify it exists:
```bash
cat /etc/tmpfiles.d/chrote-tmux.conf
# Should contain:
# d /run/tmux 0755 root root -
# d /run/tmux/chrote 0700 chrote chrote -
```

#### Can't create new sessions

```bash
# Check user
whoami  # Should be chrote

# Check permissions
ls -la /run/tmux/chrote/

# Try creating manually
TMUX_TMPDIR=/run/tmux/chrote tmux new-session -d -s test
```

---

### Service Issues

#### chrote-server won't start

```bash
# Check status
systemctl status chrote-server

# Check logs
journalctl -u chrote-server -n 50

# Common issues:
# - Port 8080 already in use
# - Binary not found
# - Permission denied

# Check if port is in use
ss -tlnp | grep 8080

# Verify binary exists
ls -la /home/chrote/chrote-server
```

#### chrote-ttyd won't start

```bash
# Check status
systemctl status chrote-ttyd

# Check logs
journalctl -u chrote-ttyd -n 50

# Verify ttyd binary
ls -la /usr/local/bin/ttyd
/usr/local/bin/ttyd --version
```

#### Services start but crash immediately

Check for startup errors:
```bash
journalctl -u chrote-server -f &
sudo systemctl restart chrote-server
# Watch for error messages
```

---

### Build Issues

#### npm run build fails

```bash
# Clear cache and reinstall
cd /code/dashboard
rm -rf node_modules package-lock.json
npm install
npm run build
```

#### go build fails

```bash
# Check Go version
/usr/local/go/bin/go version  # Need 1.23+

# Clear Go cache
go clean -cache

# Rebuild
cd /code/src
go build -o ~/chrote-server ./cmd/server
```

#### TypeScript errors

```bash
# Run type check
cd /code/dashboard
npx tsc --noEmit

# Fix errors before building
```

---

### Network Issues

#### Can't access chrote:8080 remotely

1. **Is Tailscale running?**
   ```bash
   tailscale status
   ```

2. **Is the hostname correct?**
   ```bash
   tailscale status | grep chrote
   ```

3. **Is the service listening on all interfaces?**
   The Go server listens on `:8080` which means all interfaces. This should work.

4. **Firewall issues?**
   WSL2 usually doesn't have firewall issues, but check:
   ```bash
   sudo iptables -L
   ```

#### localhost works but chrote:8080 doesn't

This is a Tailscale/DNS issue:
```bash
# Check Tailscale is up
tailscale status

# Check hostname resolution
ping chrote

# Re-authenticate if needed
sudo tailscale up --hostname chrote
```

---

### ChroteChat Issues

#### Messages not sending

```bash
# Check chat API
curl http://localhost:8080/api/chat/status

# Check if gt (gastown) is in PATH
which gt
gt --version

# Check workspace path
echo $PWD  # Should be /code or /home/chrote/chrote
```

#### Nudge not reaching agents

The nudge feature requires:
1. A running tmux session with that name
2. The session running Claude Code (for mail to work)
3. gt tool installed and in PATH

```bash
# List sessions
TMUX_TMPDIR=/run/tmux/chrote tmux list-sessions

# Test nudge manually
gt nudge session-name "test message"
```

---

## Quick Reference Commands

### Status Checks
```bash
systemctl status chrote-server chrote-ttyd
curl http://localhost:8080/api/health
TMUX_TMPDIR=/run/tmux/chrote tmux list-sessions
```

### Restart Everything
```bash
sudo systemctl restart chrote-server chrote-ttyd
```

### View Logs
```bash
journalctl -u chrote-server -f          # Server logs
journalctl -u chrote-ttyd -f            # Terminal logs
journalctl -u chrote-server -u chrote-ttyd -f  # Both
```

### Full Rebuild (Nuclear Option)
```bash
cd /code/dashboard
rm -rf node_modules dist
npm ci
npm run build
cp -r dist/* ../src/internal/dashboard/
cd ../src
go clean -cache
go build -o ~/chrote-server ./cmd/server
sudo systemctl restart chrote-server chrote-ttyd
```

### WSL Reset
```powershell
# From Windows PowerShell
wsl --shutdown
# Wait 10 seconds
.\Chrote-Toggle.ps1
```

---

## For Claude: Troubleshooting Checklist

When a user reports an issue, follow this diagnostic flow:

### 1. Identify the Environment
Ask: "Are you viewing localhost:5173 or localhost:8080?"
- 5173 = Vite dev server (frontend changes are instant)
- 8080 = Production Go server (requires rebuild)

### 2. Check Service Status
```bash
systemctl status chrote-server chrote-ttyd
```

### 3. Check Logs for Errors
```bash
journalctl -u chrote-server -n 20
```

### 4. Verify the Fix Actually Applied
- **Frontend change in dev mode**: Just save the file, check browser
- **Frontend change in prod mode**: Must rebuild npm, copy dist, rebuild Go, restart service
- **Backend change**: Must rebuild Go binary, restart service

### 5. Common Misconceptions to Correct
- "I restarted the server but nothing changed" → Check if they're in dev mode
- "The build succeeded but it's not working" → Check if they copied dist/ and restarted
- "Works on 8080 but not 5173" → Check vite.config.ts proxy settings
- "Works on 5173 but not 8080" → Check if Go binary was rebuilt

### 6. Nuclear Options (When Nothing Else Works)
```bash
# Full clean rebuild
cd /code/dashboard && rm -rf node_modules dist && npm ci && npm run build
cp -r dist/* ../src/internal/dashboard/
cd ../src && go clean -cache && go build -o ~/chrote-server ./cmd/server
sudo systemctl restart chrote-server chrote-ttyd
```

```powershell
# WSL reset (from Windows)
wsl --shutdown
.\Chrote-Toggle.ps1
```
