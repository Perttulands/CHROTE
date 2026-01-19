# CHROTE

![CHROTE](CHROTE.png)

**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution

---

> **WARNING:** This software was vibe-coded at 3am by mass hallucinations between a human and multiple AI agents. It works on my machine. It might work on yours. It probably won't. If it does, that's the miracle - not the expectation.

> **DANGER:** You are about to run untested code that spawns dozens of AI agents with terminal access. They will read your files. They will write your files. They will argue with each other. They will occasionally achieve something useful. Mostly they will burn through your API credits like a war rig burns guzzoline.

> **CAUTION:** If you have to ask "is this safe?" - turn back now. This is not safe. This is the wasteland. We don't do safe here. We do *fast*, *loud*, and *pray the tests pass*.

---

## What Is This?

CHROTE is a web dashboard for running swarms of AI coding agents via tmux sessions. It's the control room for your wasteland coding operation.

You know how normal people run one Claude Code instance and carefully review each change?

We don't do that here.

Here, we spin up 10, 20, 30 agents. We point them at problems. We watch the chaos unfold through terminal windows. Sometimes they solve the problem. Sometimes they fight each other. Sometimes they all independently decide to refactor the same file and create merge conflicts that would make God weep.

**It's beautiful.**

![Dashboard Screenshot](screenshot%201.png)

---

## Who Is This For?

**This is NOT for you if:**
- You've never used Claude Code
- You think "vibe coding" sounds irresponsible
- You have a budget
- You need things to work reliably
- You value your sanity

**This IS for you if:**
- You're already mass-prompting Claude Code instances in a dozen terminal tabs
- You've accepted that AI will write most of your code and you're just here to steer
- You understand that "it works on my machine" is a lifestyle
- You have more API credits than sense
- You want to feel like a mad scientist running a robot army

---

## The Crew

Every terminal window in CHROTE has a guardian - a wasteland operator watching over your agents. They don't actually *do* anything. They're just... there. Staring. Judging your tmux sessions with silent, pixel-based disapproval. Think of them as the dashboard's emotional support animals, except they're cyberpunk rodents who've seen some shit and have zero therapeutic credentials.

Meet them:

### Terminal 1 - The Veterans

<table>
<tr>
<td width="25%" align="center">
<img src="dashboard/public/bg-polecat.png" width="150"><br>
<b>POLECAT</b><br>
<i>The Mechanic</i><br>
V8 engine heart. Keeps the rigs running when everything's on fire.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg_fox.png" width="150"><br>
<b>FOX</b><br>
<i>The Strategist</i><br>
Monocle and military precision. Plans the operations others execute.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg-badger.png" width="150"><br>
<b>BADGER</b><br>
<i>The Engineer</i><br>
Welding goggles and steady hands. Builds what Fox designs.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg_wolf.png" width="150"><br>
<b>WOLF</b><br>
<i>The Enforcer</i><br>
Hooded and chained. When sessions need killing, Wolf answers.
</td>
</tr>
</table>

### Terminal 2 - The Operations

<table>
<tr>
<td width="25%" align="center">
<img src="dashboard/public/bg_crew.png" width="150"><br>
<b>CREW</b><br>
<i>The Technician</i><br>
Wrench in hand, plasma flowing. Keeps the terminals alive.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg_convoy.png" width="150"><br>
<b>CONVOY</b><br>
<i>The Transport</i><br>
The war rig itself. Carries your workloads across the wasteland.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg_hawk.png" width="150"><br>
<b>HAWK</b><br>
<i>The Architect</i><br>
Cloaked scholar. Reads the ancient docs. Guides the workers.
</td>
<td width="25%" align="center">
<img src="dashboard/public/bg_town.png" width="150"><br>
<b>TOWN</b><br>
<i>The Settlement</i><br>
CHROTE itself. The glowing hub where all roads lead.
</td>
</tr>
</table>

---

## The Soundtrack

Every wasteland operation needs its anthem. The official CHROTE soundtrack - lo-fi beats for coding robots:

**[CHROTE Official Playlist](https://suno.com/playlist/8bbca04c-31de-4f6b-a989-372cfd73b382)**

Listen to that shit. You might learn something. The lyrics explain you Gastown and Tmux.

**Built-in tracks** (yes, we ship MP3s with the codebase - this is the wasteland, might as well have some tunes while you watch a stream of errors). These are instrumental versions. No lyrics. Just vibes. Because apparently the AI thought your terminal sessions didn't need a narrator explaining what was happening. Bold choice. We kept it.

- **Design_phase** - When the architects are scheming
- **Who_ate_my_PRD** - The eternal question
- **Vibes_at_the_hq** - Command center ambience
- **Mayors_introspection** - Deep thoughts from the control room
- **The_idle_Polecat** - When the mechanic takes a smoke break
- **Polecat_Danceparty** - V8 engine rhythms
- **Deacons_revenge** - Things got personal
- **MergePush** - The moment of truth
- **March_of_the_Polecats** - When the crew mobilizes
- **Convoy_Run** - Full throttle across the wasteland

Access the music player in the dashboard tab bar. Let it play while your agents "work". They can't hear it, but you'll feel better.

---

## Deployment Protocol

> **STOP.** Before you proceed, accept these truths:
> 1. This will probably break
> 2. When it breaks, you get to keep both pieces
> 3. There is no support team. There is only the wasteland.

### Prerequisites

- Windows 11 with WSL2 (we don't do Docker here, too many layers of abstraction between us and the metal)
- Ubuntu 24.04 in WSL
- Tailscale account (because exposing this to the internet would be *insane*)
- A reckless disregard for best practices

### Installation

**Two commands. That's it.**

```powershell
# 1. Install Ubuntu in WSL (skip if you already have it)
wsl --install -d Ubuntu-24.04

# 2. Run setup from the CHROTE directory
cd C:\path\to\CHROTE   # wherever you cloned/extracted it
.\Chrote-Toggle.ps1 -Setup
```

The `-Setup` flag auto-detects your Ubuntu distro and handles CRLF line endings automatically. No sudo needed. No password prompts. Just run and walk away.

**Manual alternative** (if PowerShell isn't your thing):
```powershell
wsl -d Ubuntu-24.04 -u root -e bash -c "tr -d '\r' < /mnt/c/path/to/CHROTE/wsl/setup-wsl.sh | bash"
```

> **ZIP Download Users:** If you downloaded this as a ZIP (not via Git), Windows may have added CRLF line endings to the scripts. Both methods above handle this automatically by stripping CRLF before execution. You can also use `wsl/bootstrap.sh` as an alternative entry point.

> **Mac users:** I used to have a Mac. Then I inhaled too many Sharpies and went out and bought a Windows computer. Don't be like me. But what that *does* mean is that CHROTE is Windows-native and you'll need to do some cooking to get it running on macOS. The core concepts translate - you've got native Linux, you've got tmux, you just need to wire up the systemd services differently. PRs welcome. We don't judge. Much.

> **Path flexibility:** The setup script automatically detects where CHROTE is located. Clone it anywhere you want - `C:\Users\you\CHROTE`, `D:\Projects\CHROTE`, wherever. Just run `.\Chrote-Toggle.ps1 -Setup` from that directory.

### What Gets Installed

The setup script (`wsl/setup-wsl.sh`) handles everything:

| Component | What It Does |
|-----------|--------------|
| **WSL Config** | Enables systemd, sets default user to `chrote` |
| **chrote user** | Non-root user for running agents (no sudo) |
| **Dependencies** | curl, git, tmux, python3, jq, rsync, build-essential |
| **Go 1.23** | For building the server and tools |
| **Node.js 20** | For building the dashboard |
| **ttyd** | Web terminal backend |
| **Claude Code** | Anthropic's CLI (via npm) |
| **CHROTE Server** | Go binary serving the dashboard |
| **Vendored Tools** | gastown (gt), beads (bd), beads_viewer (bv) |
| **systemd Services** | chrote-server and chrote-ttyd auto-start on boot |

After installation, the following paths are available inside WSL:

| Path | Purpose |
|------|---------|
| `/code` | Symlink to `~/chrote` (copy of your CHROTE install) |
| `/vault` | Symlink to E:\Vault (optional, for read-only storage) |
| `~/.local/bin/gt` | Gastown orchestrator |
| `~/.local/bin/bd` | Beads issue tracker |
| `~/.local/bin/bv` | Beads viewer |

### Ignition

```powershell
# The toggle script - your daily driver
.\Chrote-Toggle.ps1          # Start and open browser
.\Chrote-Toggle.ps1 -Setup   # Run first-time setup
.\Chrote-Toggle.ps1 -Stop    # Kill everything
.\Chrote-Toggle.ps1 -Status  # Check if anything's alive
.\Chrote-Toggle.ps1 -Logs    # Watch the chaos unfold
```

### Verifying Installation

After setup completes and WSL restarts:

```powershell
# Check if services are running
.\Chrote-Toggle.ps1 -Status

# You should see:
# WSL: Running
# chrote-server: active (running)
# chrote-ttyd: active (running)
# API: OK
```

If something's wrong:
```powershell
# Check the logs
.\Chrote-Toggle.ps1 -Logs
```

### Access Points

Once the rig is running (IF the rig is running):

| Outpost | Location | Purpose |
|---------|----------|---------|
| Command Center | `http://localhost:8080` | Main dashboard (local access) |
| Command Center | `http://chrote:8080` | Main dashboard (via Tailscale) |
| Direct Terminal | `http://localhost:8080/terminal/` | Raw ttyd access - for when the UI fails |
| File Depot | `http://localhost:8080/api/files/` | File API - surprisingly stable |

> **Note:** `http://chrote:8080` requires Tailscale configured on your WSL instance. For initial testing, use `http://localhost:8080` which works immediately.

### Setting Up Tailscale (Optional but Recommended)

For remote access from other devices:

```bash
# Inside WSL
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --hostname chrote
```

Then access from any device on your Tailnet via `http://chrote:8080`.

---

## The Gastown Connection

CHROTE is infrastructure. **Gastown** is what runs on it.

Gastown is Steve Yegge's orchestration framework for running 10-30+ AI coding agents in parallel. CHROTE gives Gastown a home - terminals to run in, a dashboard to monitor, and a "Nuke All" button for when everything goes wrong (which is often).

### Getting Started with Gastown

After CHROTE is installed, connect to WSL and start orchestrating:

```bash
# Enter WSL (auto-logs in as chrote user)
wsl

# Check that tools are installed
which gt bd bv   # Should show ~/.local/bin paths

# Start the gastown orchestrator
gt start gastown

# Check status
gt status

# Peek at what agents are doing
gt peek
```

### The Workflow

- **Beads** - atomic units of work (issues, tasks)
- **Epics** - collections of parallel tasks
- **Molecules** - complex workflow chains
- **Wisps** - ephemeral coordination tasks

The philosophy: **Physics over Politeness**. Sessions are expendable. Throughput is the mission. The "Nuke All" button isn't a failure state - it's a feature. Burn it down, start fresh, try again.

```bash
# Inside WSL - start the machine
gt start gastown
gt status
gt peek
```

---

## Dashboard Controls

### Terminal View
- 1-4 terminal panes per tab (two tabs = 8 total windows)
- Drag sessions from sidebar onto windows
- Click tabs to switch between assigned sessions
- Each window has its guardian watching over your agents

![Beads View](screenshot%202.png)

### Files and Themes

Access your project files directly through the dashboard with the native file browser:

![File Browser](file%20system.png)

Customize your workspace with multiple built-in themes:

![Themes](Themes.png)

### The Nuclear Option

See that "Nuke All Sessions" button?

It does exactly what it says. All sessions. Gone. Instantly.

Use it liberally. This is the wasteland. Attachment is weakness. If your agents are stuck in loops, arguing with themselves, or have collectively decided to rewrite your codebase in Haskell - nuke them. Start over. You'll feel better.

### Session Naming

| Prefix | Example | What It Means |
|--------|---------|---------------|
| `hq-` | `hq-mayor` | Headquarters - coordination sessions |
| `gt-rigname-` | `gt-gastown-jack` | Rig workers - the agents doing actual work |
| `main`, `shell` | `main` | Your personal sessions |
| Other | `chaos-monkey` | Whatever you want, we don't judge |

---

## Architecture (For The Brave)

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Tailscale Network                             │
│              (The only thing between you and disaster)               │
└───────────────────────────────────────────────────────────────────┬──┘
                                                                    │
                    ┌───────────────────────────────────────────────▼──────────────────────────────┐
                    │                    WSL2 (Ubuntu 24.04)                                        │
                    │                   User: chrote (no sudo, we're not THAT reckless)            │
                    │                                                                              │
                    │  ┌─────────────────────────────────────────────────────────────────────┐    │
                    │  │                        systemd services                              │    │
                    │  │  ┌─────────────────────────┐  ┌─────────────────────────┐          │    │
                    │  │  │ chrote-server :8080     │  │ chrote-ttyd :7681       │          │    │
                    │  │  │ (Go binary - fast,      │  │ (web terminal - when    │          │    │
                    │  │  │  surprisingly stable)   │  │  you need raw access)   │          │    │
                    │  │  └─────────────────────────┘  └─────────────────────────┘          │    │
                    │  └─────────────────────────────────────────────────────────────────────┘    │
                    │                                                                              │
                    │  Tailscale hostname: chrote                                                  │
                    └──────────────────────────────────────────────────────────────────────────────┘
```

**Why WSL2 instead of Docker?**

Because we already tried Docker. The layers of indirection made debugging a nightmare. WSL2 gives us real Linux with real systemd and real performance. It's closer to the metal. In the wasteland, you want to feel the road.

---

## Security (Such As It Is)

> **CRITICAL:** Do not expose port 8080 to the public internet. Ever. Under any circumstances. This dashboard has no authentication. Anyone who can reach it can spawn terminals, read your files, and run commands. It's protected by Tailscale. If you bypass Tailscale, you deserve what happens next.

The security model:
- Tailscale network = perimeter
- Everything inside = trusted
- Agents run as `chrote` user = no root, limited blast radius
- File access limited to `/code` and `/vault`

See [SECURITY.md](SECURITY.md) if you want to pretend this is enterprise software.

---

## When Things Go Wrong

### Sessions disappear
```bash
echo $TMUX_TMPDIR  # Should be /run/tmux/chrote
ls -la /run/tmux/chrote/  # Check socket exists
# If not, something's very wrong. Good luck.
```

### Services won't start
```bash
systemctl status chrote-server chrote-ttyd
journalctl -u chrote-server -f
# Read the logs. The answer is in there. It's always in the logs.
```

### Terminal shows black screen
```bash
systemctl status chrote-ttyd
# Restart it
sudo systemctl restart chrote-ttyd
# Still black? Check if tmux is even running
tmux list-sessions
```

### Everything is on fire
```bash
# The nuclear option
wsl --shutdown
# Wait 10 seconds
# Start fresh
.\Chrote-Toggle.ps1
```

---

## Development

### Where's My Code?

Setup copies your files into WSL. Here's where everything lives:

| What | Where |
|------|-------|
| Windows source | `E:\Docker\CHROTE` (or wherever you cloned it) |
| WSL working copy | `/home/chrote/chrote` (aka `/code`) |
| Windows path to WSL | `\\wsl$\Ubuntu-24.04\home\chrote\chrote` |

**Edit directly in WSL from Windows** - open `\\wsl$\Ubuntu-24.04\home\chrote\chrote` in Explorer or VS Code. Changes are immediate, no sync needed.

Or use VS Code's WSL extension:
1. Install "WSL" extension
2. `Ctrl+Shift+P` → "WSL: Connect to WSL"
3. Open folder `/home/chrote/chrote`

### Making Changes

```bash
# Enter WSL
wsl

# You're now in /code as chrote user
# Edit files, then rebuild:

# Dashboard only (React + TypeScript)
cd dashboard
npm run build
cp -r dist/* ../src/internal/dashboard/
sudo systemctl restart chrote-server

# For live development with hot reload
npm run dev    # localhost:5173
```

### Running Tests

```bash
cd /code/dashboard
npm run test
# If they pass, you probably broke the tests
```

---

## File Structure

```
CHROTE/
├── src/                      # Go server (the stable part)
├── dashboard/                # React UI (the pretty part)
│   └── public/               # Guardian images live here
├── wsl/                      # WSL setup scripts (the scary part)
├── vendor/                   # Gastown, Beads (optional chaos)
├── docs/                     # Documentation (optimistic)
└── test-sessions.sh          # Creates fake sessions for testing
```

---

## Philosophy

**This is not production software.** This is a weapon. A tool for those who have decided that shipping fast matters more than shipping safe. That iteration speed beats code review. That 30 agents writing code simultaneously is better than one agent writing code carefully.

Is this responsible? No.
Is this sustainable? Probably not.
Is this the future? ...maybe.

We're all figuring this out together. The AI coding paradigm is evolving weekly. What works today might be obsolete tomorrow. What seems crazy now might be standard practice in a year.

CHROTE is a bet. A bet that the way to navigate this chaos is to embrace it. To build tools that let us run more agents, faster, with less friction. To accept that most of what they produce will be garbage, but the 10% that works will be worth it.

Welcome to the wasteland.
Keep your API key loaded.
And remember: when in doubt, nuke it and start over.

---

## See Also

| Document | What It Is |
|----------|------------|
| [PRD.md](PRD.md) | What we thought we were building |
| [SPEC.md](SPEC.md) | What we actually built |
| [SECURITY.md](SECURITY.md) | How we pretend this is secure |

---

## License

MIT - Because even in the wasteland, we believe in open source.

---

<p align="center">
<i>"I live, I die, I live again!"</i><br>
<small>- Every tmux session, probably</small>
</p>
