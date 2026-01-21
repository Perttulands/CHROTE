# ChroteChat Mail Integration

ChroteChat uses the Gastown mail system (`gt mail`) for message delivery. This document explains how the mail system works and how ChroteChat integrates with it.

## Overview

ChroteChat uses a **dual-channel delivery** system:
1. **Mail** (persistence) - Messages stored in beads database, persist until read
2. **Nudge** (real-time) - Signals agent to check their inbox immediately

## Gastown Mail System

### Storage Model

All mail messages are stored as **beads issues with `type=message`** in the town-level beads database (`{townRoot}/.beads`).

```
Town (.beads/)
├── mayor/              ← Mayor inbox
└── gastown/            ← Rig mailboxes
    ├── witness         ← rig/witness
    ├── refinery        ← rig/refinery
    ├── Toast           ← rig/Toast (polecat)
    └── crew/max        ← rig/crew/max (crew worker)
```

### Address Formats

| Address | Description |
|---------|-------------|
| `mayor/` | Send to Mayor |
| `<rig>/refinery` | Send to a rig's Refinery |
| `<rig>/<polecat>` | Send to a specific polecat (e.g., `greenplace/Toast`) |
| `<rig>/` | Broadcast to a rig |
| `list:<name>` | Send to a mailing list (fans out to all members) |

Mailing lists are defined in `~/gt/config/messaging.json` and allow sending to multiple recipients at once.

### Message Types

| Type | Description |
|------|-------------|
| `task` | Required processing |
| `scavenge` | Optional first-come work |
| `notification` | Informational (default) |
| `reply` | Response to message |

### Priority Levels

| Priority | Description |
|----------|-------------|
| 0 | urgent/critical |
| 1 | high |
| 2 | normal (default) |
| 3 | low |
| 4 | backlog |

### Message Structure

Messages in beads have:
- `title` → Subject
- `description` → Body
- `assignee` → Recipient identity
- `priority` → 0=urgent, 1=high, 2=normal, 3=low, 4=backlog
- `status` → open (unread) or closed (read/archived)
- `labels` → Metadata (from:X, thread:X, reply-to:X, etc.)

### gt mail send Command

```
gt mail send <address> [flags]

Flags:
      --cc stringArray    CC recipients (can be used multiple times)
  -h, --help              help for send
  -m, --message string    Message body
  -n, --notify            Send tmux notification to recipient
      --permanent         Send as permanent (not ephemeral, synced to remote)
      --pinned            Pin message (for handoff context that persists)
      --priority int      Message priority (0-4) (default 2)
      --reply-to string   Message ID this is replying to
      --self              Send to self (auto-detect from cwd)
  -s, --subject string    Message subject (required)
      --type string       Message type (task, scavenge, notification, reply) (default "notification")
      --urgent            Set priority=0 (urgent)
      --wisp              Send as wisp (ephemeral, default) (default true)
```

### Examples

```bash
# Status check
gt mail send greenplace/Toast -s "Status check" -m "How's that bug fix going?"

# Report to mayor
gt mail send mayor/ -s "Work complete" -m "Finished gt-abc"

# Broadcast to rig
gt mail send gastown/ -s "All hands" -m "Swarm starting" --notify

# Task with priority
gt mail send greenplace/Toast -s "Task" -m "Fix bug" --type task --priority 1

# Urgent message
gt mail send greenplace/Toast -s "Urgent" -m "Help!" --urgent

# Reply to message
gt mail send mayor/ -s "Re: Status" -m "Done" --reply-to msg-abc123

# Send to self (handoff)
gt mail send --self -s "Handoff" -m "Context for next session"

# CC recipient
gt mail send greenplace/Toast -s "Update" -m "Progress report" --cc overseer

# Send to mailing list
gt mail send list:oncall -s "Alert" -m "System down"
```

### gt nudge Command

Universal synchronous messaging API for Gas Town worker-to-worker communication. Delivers a message directly to any worker's Claude Code session.

Uses a reliable delivery pattern:
1. Sends text in literal mode (-l flag)
2. Waits 500ms for paste to complete
3. Sends Enter as a separate command

This is the ONLY way to send messages to Claude sessions. Do not use raw tmux send-keys elsewhere.

```
gt nudge <target> [message] [flags]

Flags:
  -f, --force            Send even if target has DND enabled
  -h, --help             help for nudge
  -m, --message string   Message to send
```

**Role shortcuts** (expand to session names):
| Shortcut | Expands to |
|----------|------------|
| `mayor` | `gt-mayor` |
| `deacon` | `gt-deacon` |
| `witness` | `gt-<rig>-witness` (uses current rig) |
| `refinery` | `gt-<rig>-refinery` (uses current rig) |

**Channel syntax:**
`channel:<name>` nudges all members of a named channel defined in `~/gt/config/messaging.json` under `nudge_channels`. Patterns like `gastown/polecats/*` are expanded.

**DND (Do Not Disturb):**
If the target has DND enabled (`gt dnd on`), the nudge is skipped. Use `--force` to override.

**Examples:**
```bash
gt nudge greenplace/furiosa "Check your mail and start working"
gt nudge greenplace/alpha -m "What's your status?"
gt nudge mayor "Status update requested"
gt nudge witness "Check polecat health"
gt nudge deacon session-started
gt nudge channel:workers "New priority work available"
```

## ChroteChat Integration

### Backend Flow (chat.go)

When a user sends a chat message:

```go
// 1. Send via Mail (Persistence)
mailCmd := exec.Command("gt", "mail", "send", target, "-s", "Chat Message", "-m", message)
mailCmd.Dir = workspace  // Must run from Gastown workspace

// 2. Nudge (Real-time attention)
nudgeCmd := exec.Command("gt", "nudge", target, "New chat message")
nudgeCmd.Dir = workspace
```

### Workspace Detection

ChroteChat automatically detects the Gastown workspace by:
1. Getting tmux session paths via `tmux list-sessions -F "#{session_name}|#{session_path}"`
2. Walking up from each session path to find a directory with `daemon/`
3. Associating the workspace with each conversation

Example:
```
Session: hq-mayor
Path: /code/Chrotetown
Workspace: /home/chrote/workspace/Chrotetown (has daemon/)
```

### Target Resolution

The `target` for mail commands is derived from tmux session names:

| Session Name | Target | Example |
|--------------|--------|---------|
| `hq-mayor` | `mayor/` | `gt mail send mayor/ -s "..." -m "..."` |
| `hq-deacon` | `deacon/` | `gt mail send deacon/ -s "..." -m "..."` |
| `gt-Chrote-witness` | `Chrote/witness` | `gt mail send Chrote/witness -s "..." -m "..."` |
| `gt-Chrote-refinery` | `Chrote/refinery` | `gt mail send Chrote/refinery -s "..." -m "..."` |
| `gt-Chrote-crew-Ronja` | `Chrote/crew/Ronja` | `gt mail send Chrote/crew/Ronja -s "..." -m "..."` |
| `gt-Chrote-jasper` | `Chrote/jasper` | `gt mail send Chrote/jasper -s "..." -m "..."` |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/chat/conversations` | GET | List available chat targets with workspace info |
| `/api/chat/history` | GET | Get message history for a target |
| `/api/chat/send` | POST | Send message via mail + nudge |
| `/api/chat/nudge` | POST | Send nudge only (quick ping, no mail) |
| `/api/chat/workspaces` | GET | List detected Gastown workspaces |
| `/api/chat/session/status` | GET | Check if chrote-chat session exists |
| `/api/chat/session/init` | POST | Initialize chrote-chat session in workspace |
| `/api/chat/session/restart` | POST | Restart chrote-chat session |

### Send Request

```json
POST /api/chat/send
{
  "workspace": "/home/chrote/workspace/Chrotetown",
  "target": "mayor",
  "message": "Hello from the dashboard!"
}
```

### Send Response

```json
{
  "success": true,
  "data": {
    "success": true,
    "messageId": "msg-1234567890",
    "mailSent": true,
    "nudged": true
  }
}
```

## Reading Mail (Agent Side)

### gt mail inbox

Lists messages in your inbox (unread messages addressed to you).

```bash
gt mail inbox
```

### gt mail read

Read a specific message (does not mark as read).

```bash
gt mail read <message-id>
gt mail show <message-id>  # alias
```

The message ID can be found from `gt mail inbox`.

### gt mail mark-read

Mark a message as read (moves to closed status).

```bash
gt mail mark-read <message-id>
```

### gt mail check

Check for new mail (used by hooks).

```bash
gt mail check
```

## Chat History Implementation

Chat history is implemented by querying the beads mail system:

1. **Sent messages**: Query the target's inbox (`gt mail inbox <target> --json`)
2. **Received messages**: Query the overseer's inbox (`gt mail inbox overseer --json`)
3. **Merge and sort**: Combine both, sort chronologically, label with role (user/agent)

The `/api/chat/history` endpoint accepts `target` and `workspace` query parameters and returns messages from both mailboxes.

## Appendix: tmux send-keys

The underlying mechanism for `gt nudge` is tmux's `send-keys` command, which pushes strings (terminal commands) into tmux sessions.

### Basic Syntax

```bash
tmux send-keys -t <target> "command" Enter
```

### Options

| Flag | Description |
|------|-------------|
| `-t <target>` | Target session, window, or pane |
| `-l` | Send keys literally (disables special key name lookup) |
| `-R` | Reset terminal state before sending |

### Examples

```bash
# Send to a specific session
tmux send-keys -t my-session "ls -la" Enter

# Target a specific window/pane
tmux send-keys -t my-session:0.0 "echo hello" Enter

# Using C-m instead of Enter
tmux send-keys -t my-session 'git status' C-m

# Literal mode (no special key interpretation)
tmux send-keys -l -t my-session "some text"
```

### Special Keys

You can send special keys: `Enter`, `C-m` (carriage return), `Up`, `Down`, `Left`, `Right`, `Escape`, `Tab`, `Space`, `BSpace` (backspace), `F1`-`F12`, etc.

### Sending from Outside tmux

The `send-keys` command works from any shell—you don't need to be inside tmux. The only requirement is that the tmux server is running and accessible.

**Default socket:** tmux uses `/tmp/tmux-$UID/default` by default.

**Custom socket:** If sessions use a different socket (like chrote uses `/run/tmux/chrote/`), specify it with `-S`:

```bash
# Specify socket path explicitly
tmux -S /run/tmux/chrote/default send-keys -t my-session "command" Enter
```

Or set `TMUX_TMPDIR` so tmux finds the right socket automatically.

**Target format:** `session:window.pane`

| Target | Description |
|--------|-------------|
| `my-session` | First window, active pane |
| `my-session:2` | Window index 2, active pane |
| `my-session:2.0` | Window 2, pane 0 |
| `my-session:name` | Window named "name" |

### Why gt nudge Exists

While raw `tmux send-keys` works, `gt nudge` provides a reliable delivery pattern specifically for Claude Code sessions:
1. Sends text in literal mode (`-l` flag) to avoid special character interpretation
2. Waits 500ms for paste to complete
3. Sends Enter as a separate command

This prevents issues with fast typing, special characters, and ensures the message is fully received before submission.

## Testing

E2E tests verify the complete flow:

```bash
cd src && go test -v ./internal/api/ -run TestChatE2E -tags=e2e
```

Tests verify:
- Workspace detection from tmux sessions
- Conversations include workspace path
- SendMessage validation (workspace, target, message required)
- Full send flow with `gt mail send` and `gt nudge`
