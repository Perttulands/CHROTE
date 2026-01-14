# Arena Security Guide

## Threat Model

**Primary concern:** A compromised arena container could pivot to attack other devices on your Tailnet (NAS, other machines, home services).

**Secondary concern:** The sandbox itself is throw-away, so protecting it is lower priority.

## Critical Protection: Tailscale ACL Lockdown

### The Problem

Without ACLs, your containers are full Tailnet members that can reach **all your other devices**. A compromised arena could:
- Scan your entire Tailnet (`tailscale status` reveals all devices)
- SSH to other machines
- Access file shares, NAS, home automation, etc.
- Potentially attack Landmass (the Docker host)

### The Solution

Isolate containers using **tagged auth keys** and **ACLs** so they can only reach:
- Each other (arena <-> ollama)
- The internet (npm, pip, git, API calls)
- **NOT** your personal Tailnet devices

---

## Implementation Steps

### Step 1: Create Tagged Auth Key

1. Go to https://login.tailscale.com/admin/settings/keys
2. Click **Generate auth key**
3. Configure:
   - **Reusable**: Yes
   - **Ephemeral**: No (you want persistent hostname identity)
   - **Tags**: Add `tag:container`
4. Copy the new key

### Step 2: Configure Tailscale ACLs

Go to https://login.tailscale.com/admin/acls

If you have existing ACLs, merge these rules. If starting fresh, use this:

```json
{
  "tagOwners": {
    "tag:container": ["autogroup:admin"]
  },
  "grants": [
    // Your personal devices can reach everything (including containers)
    {"src": ["autogroup:member"], "dst": ["*"], "ip": ["*"]},

    // Containers can reach each other (arena <-> ollama)
    {"src": ["tag:container"], "dst": ["tag:container"], "ip": ["*"]},

    // Containers can reach the internet (npm, pip, git, APIs)
    {"src": ["tag:container"], "dst": ["autogroup:internet"], "ip": ["*"]}

    // IMPLICIT DENY: containers CANNOT reach your other Tailnet devices
  ]
}
```

### Step 3: Update .env and Recreate Containers

```powershell
cd E:\Docker\AgentArena

# Edit .env - replace auth key with the new tagged one
# TS_AUTHKEY=tskey-auth-XXXXX-YYYYYYYY

# Tear down and clear old Tailscale identities
docker-compose down
Remove-Item -Recurse -Force .\tailscale_state\arena\*
Remove-Item -Recurse -Force .\tailscale_state\ollama\*

# Bring containers back up
docker-compose up -d
```

### Step 4: Verify in Tailscale Admin

Check https://login.tailscale.com/admin/machines

You should see:
- `arena` with tag `tag:container`
- `ollama` with tag `tag:container`

### Step 5: Verify Isolation

```bash
# SSH into arena
ssh dev@arena

# Should SUCCEED - container-to-container communication:
curl http://ollama:11434/api/tags

# Should SUCCEED - internet access:
curl -I https://google.com

# Should FAIL - cannot reach other Tailnet devices:
ping <ip-of-your-nas-or-other-device>
# Expected: timeout or "Destination unreachable"
```

---

## What This Achieves

| Source | Can Reach | Cannot Reach |
|--------|-----------|--------------|
| Your laptop/phone | arena, ollama, all your devices | - |
| arena (even if compromised) | ollama, internet | NAS, Landmass host, other devices |
| ollama | arena, internet | NAS, Landmass host, other devices |

---

## Running Agents with Bypass Permissions

Once ACLs are configured, running this inside arena is acceptable:

```bash
claude --dangerously-skip-permissions
```

**Why it's safe:**
- Container is network-isolated from your other Tailnet devices
- Only writable area is `/code` (your working directory)
- `/vault` is read-only for the dev user
- Worst case scenario: rebuild the container

---

## Accessing Dev Servers

From any device on your Tailnet:

| URL | Purpose |
|-----|---------|
| `http://arena:3000` | Vite, React, Next.js dev servers |
| `http://arena:5000` | Flask, Python apps |
| `http://arena:8000` | FastAPI, Express |
| `http://arena:8080` | General web apps |
| `http://ollama:11434` | Ollama LLM API |

Dev servers must bind to `0.0.0.0` to be accessible:

```bash
# Vite
npm run dev -- --host 0.0.0.0

# Flask
flask run --host=0.0.0.0

# FastAPI
uvicorn main:app --host 0.0.0.0

# Next.js
npm run dev -- -H 0.0.0.0
```

---

## Secondary Considerations

### Password Authentication (dev:dev, root:root)

**Verdict: Keep it.** With ACLs in place:
- An attacker must first breach Tailscale (requires your Google 2FA)
- Even then, they're stuck in an isolated container
- Change passwords if it bothers you: `passwd dev` inside the container

### Resource Limits

Optional - add to `docker-compose.yml` to prevent runaway processes:

```yaml
agent-arena:
  deploy:
    resources:
      limits:
        cpus: '4'
        memory: 16G
```

---

## Verification Checklist

- [ ] New tagged auth key generated with `tag:container`
- [ ] ACLs configured in Tailscale admin console
- [ ] Old `tailscale_state/` contents cleared
- [ ] Containers recreated with `docker-compose up -d`
- [ ] Both machines show `tag:container` in Tailscale admin
- [ ] From arena: `curl http://ollama:11434/api/tags` succeeds
- [ ] From arena: `curl https://google.com` succeeds
- [ ] From arena: ping to other Tailnet device **fails**
- [ ] From your laptop: `ssh dev@arena` succeeds
- [ ] From your laptop: `curl http://arena:3000` succeeds (when dev server running)

---

## Quick Reference Commands

```bash
# Start everything
docker-compose up -d

# Stop everything
docker-compose down

# View logs
docker-compose logs -f

# Full rebuild (preserves volumes)
docker-compose down && docker-compose build --no-cache && docker-compose up -d

# SSH access
ssh dev@arena    # as dev user
ssh root@arena   # as root (can write to /vault)

# Check Tailscale status from inside container
docker exec -it agentarena-dev tailscale status
```
