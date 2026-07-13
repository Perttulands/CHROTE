# SCHEDULING.md - Cron Jobs System Design

Last updated: 2026-01-21

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Job Types](#2-job-types)
3. [Scheduling Mechanisms](#3-scheduling-mechanisms)
4. [UI Design](#4-ui-design)
5. [Use Cases](#5-use-cases)
6. [Error Handling](#6-error-handling)
7. [Implementation Plan](#7-implementation-plan)
8. [API Endpoints](#8-api-endpoints)
9. [Security Considerations](#9-security-considerations)

---

## 1. Architecture Overview

### 1.1 Design Decision: Custom Scheduler vs systemd Timers

| Approach | Pros | Cons |
|----------|------|------|
| **systemd timers** | OS-native, battle-tested, survives reboots | Requires root or polkit, less flexible, no UI integration |
| **Custom scheduler** | Full control, UI integration, agent-aware, git-backed persistence | Must handle reliability ourselves |

**Decision: Custom Scheduler Daemon**

Rationale:
- CHROTE runs as non-root user (`chrote`) - systemd user timers have limitations
- Deep integration with Gastown's mail system and agent orchestration
- Jobs need to interact with tmux sessions and agent hooks
- Git-backed persistence aligns with Beads architecture
- UI-first design enables browser-based job management

### 1.2 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Browser (Any Device)                          │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                    React Dashboard                             │  │
│  │  ┌──────────┬──────────┬───────┬───────┬──────────┬─────────┐ │  │
│  │  │Terminal 1│Terminal 2│ Files │ Beads │ Scheduler│ Settings│ │  │
│  │  └──────────┴──────────┴───────┴───────┴──────────┴─────────┘ │  │
│  │                                    │                           │  │
│  │                              ┌─────┴─────┐                     │  │
│  │                              │ Scheduler │                     │  │
│  │                              │    Tab    │                     │  │
│  │                              ├───────────┤                     │  │
│  │                              │ Job List  │                     │  │
│  │                              │ Create/   │                     │  │
│  │                              │ Edit Form │                     │  │
│  │                              │ History   │                     │  │
│  │                              │ Log View  │                     │  │
│  │                              └───────────┘                     │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                              │ HTTP/REST
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       WSL2 (Ubuntu 24.04)                            │
│                      User: chrote (non-root)                         │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                  systemd services                              │  │
│  │  ┌────────────────────┐  ┌────────────────────┐               │  │
│  │  │ chrote-server      │  │ chrote-scheduler   │               │  │
│  │  │ (Go API :8080)     │  │ (Go daemon :8082)  │               │  │
│  │  └─────────┬──────────┘  └─────────┬──────────┘               │  │
│  │            │                       │                           │  │
│  │            │ ┌─────────────────────┘                           │  │
│  │            ▼ ▼                                                 │  │
│  │  ┌─────────────────────────────────────────────────────────┐  │  │
│  │  │              Scheduler Core Engine                       │  │  │
│  │  │  ┌───────────┬──────────────┬────────────────────────┐  │  │  │
│  │  │  │ Job Store │ Time Wheel   │ Execution Engine       │  │  │  │
│  │  │  │ (Git/YAML)│ (in-memory)  │                        │  │  │  │
│  │  │  └───────────┴──────────────┴────────────────────────┘  │  │  │
│  │  └─────────────────────────────────────────────────────────┘  │  │
│  │                              │                                 │  │
│  │     ┌────────────────────────┼────────────────────────┐       │  │
│  │     ▼                        ▼                        ▼       │  │
│  │ ┌─────────┐           ┌───────────┐           ┌────────────┐  │  │
│  │ │  tmux   │           │ gt mail   │           │ gt nudge   │  │  │
│  │ │ sessions│           │ (mail)    │           │ (direct)   │  │  │
│  │ └─────────┘           └───────────┘           └────────────┘  │  │
│  │                                                               │  │
│  │  ┌─────────────────────────────────────────────────────────┐  │  │
│  │  │              Persistence Layer                           │  │  │
│  │  │  /home/chrote/.chrote/scheduler/                        │  │  │
│  │  │  ├── jobs.yaml          # Job definitions                │  │  │
│  │  │  ├── history/           # Execution history (git-backed) │  │  │
│  │  │  │   ├── 2026-01-21.jsonl                                │  │  │
│  │  │  │   └── ...                                             │  │  │
│  │  │  └── logs/              # Detailed job logs              │  │  │
│  │  │      └── {job-id}/                                       │  │  │
│  │  └─────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.3 Integration with tmux Sessions

The scheduler integrates with CHROTE's existing tmux infrastructure:

```
┌─────────────────────────────────────────────────────────────────┐
│                    tmux Session Targeting                        │
│                                                                  │
│  Job Target Options:                                             │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐     │
│  │ Session Name   │  │ Session Prefix │  │ Session Group  │     │
│  │ e.g. "hq-main" │  │ e.g. "gt-*"    │  │ e.g. "HQ"      │     │
│  └───────┬────────┘  └───────┬────────┘  └───────┬────────┘     │
│          │                   │                   │               │
│          └───────────────────┼───────────────────┘               │
│                              ▼                                   │
│                    ┌─────────────────┐                           │
│                    │ Session Resolver│                           │
│                    │ (at exec time)  │                           │
│                    └────────┬────────┘                           │
│                             ▼                                    │
│                    ┌─────────────────┐                           │
│                    │ tmux send-keys  │                           │
│                    │ or gt mail/nudge│                           │
│                    └─────────────────┘                           │
└─────────────────────────────────────────────────────────────────┘
```

### 1.4 Persistence Layer

Jobs are stored in YAML format with git-backed history:

```yaml
# /home/chrote/.chrote/scheduler/jobs.yaml
jobs:
  - id: morning-news
    name: Morning News Digest
    enabled: true
    schedule:
      type: cron
      expression: "0 8 * * *"
    action:
      type: prompt
      target:
        session: hq-main
      content: |
        Please compile today's morning digest:
        1. Check unread mail (gt mail inbox)
        2. Summarize any overnight agent activity
        3. Highlight blocked or failed tasks
    created: 2026-01-20T10:00:00Z
    updated: 2026-01-21T08:00:00Z

  - id: health-ping
    name: Agent Health Check
    enabled: true
    schedule:
      type: interval
      every: 15m
    action:
      type: mail
      target:
        role: witness
      subject: "Health Check"
      body: "Report current status of all polecats"
    created: 2026-01-20T10:00:00Z
```

---

## 2. Job Types

### 2.1 Terminal Commands (Bash Execution)

Execute shell commands in a tmux session.

```yaml
action:
  type: bash
  target:
    session: shell1
  command: "git pull && npm run build"
  timeout: 5m
  # Options:
  wait_for_completion: true   # Monitor for exit code
  capture_output: true        # Store stdout/stderr
```

**Execution Flow:**
```
Scheduler → tmux send-keys -t {session} "{command}" Enter
         → (optionally) Monitor for completion marker
         → Log result
```

### 2.2 Prompts to Agents

Send natural language prompts to AI agents in tmux windows.

```yaml
action:
  type: prompt
  target:
    session: gt-chrote-1
    # or:
    prefix: gt-chrote-*      # All matching sessions
    group: HQ                # All sessions in group
  content: |
    Check your hook for pending work and report status.
```

**Execution Flow:**
```
Scheduler → Resolve target session(s)
         → For each session:
            → tmux send-keys -t {session} "{content}" Enter
         → Log delivery
```

### 2.3 Mail-Based Triggers

Send Gastown mail to agents.

```yaml
action:
  type: mail
  target:
    role: mayor          # → town/mayor
    # or:
    rig: chrote          # → chrote/mayor
    agent: witness       # → chrote/witness
    # or:
    address: "chrote/polecats/jasper"  # Direct address
  subject: "Scheduled Check"
  body: |
    Please review all pending beads and report blockers.
```

**Execution Flow:**
```
Scheduler → gt mail send {target} -s "{subject}" -m "{body}"
         → Log delivery status
```

### 2.4 Nudges (Direct Messages)

Send synchronous nudges to specific agents.

```yaml
action:
  type: nudge
  target:
    agent: "chrote/witness"
  message: "Report polecat status"
```

**Execution Flow:**
```
Scheduler → gt nudge {target} "{message}"
         → Log delivery
```

### 2.5 Periodic Health Checks

Built-in job type for system health monitoring.

```yaml
action:
  type: health_check
  checks:
    - api_health         # GET /api/health
    - ttyd_reachable     # Port 7681 check
    - tmux_running       # tmux socket exists
    - beads_daemon       # Beads RPC available
    - disk_space         # /code and /home usage
  notify_on_failure:
    - mail: --human
    - toast: true
```

### 2.6 Job Type Summary

| Type | Target | Use Case |
|------|--------|----------|
| `bash` | tmux session | Run commands, scripts |
| `prompt` | tmux session(s) | Send prompts to AI agents |
| `mail` | Gastown address | Async agent communication |
| `nudge` | Gastown agent | Sync agent communication |
| `health_check` | System | Infrastructure monitoring |
| `webhook` | URL | External integrations |
| `chain` | Job IDs | Sequential job execution |

---

## 3. Scheduling Mechanisms

### 3.1 Cron Syntax Support

Standard 5-field cron expressions with extensions:

```yaml
schedule:
  type: cron
  expression: "0 8 * * 1-5"   # Weekdays at 8:00 AM
  timezone: "America/Los_Angeles"
```

**Supported Fields:**

| Field | Values | Special Characters |
|-------|--------|-------------------|
| Minute | 0-59 | * , - / |
| Hour | 0-23 | * , - / |
| Day of Month | 1-31 | * , - / L W |
| Month | 1-12 or JAN-DEC | * , - / |
| Day of Week | 0-6 or SUN-SAT | * , - / L # |

**Examples:**

| Expression | Meaning |
|------------|---------|
| `* * * * *` | Every minute |
| `0 * * * *` | Every hour |
| `0 8 * * *` | Daily at 8:00 AM |
| `0 8 * * 1-5` | Weekdays at 8:00 AM |
| `0 0 1 * *` | First day of month |
| `0 */6 * * *` | Every 6 hours |
| `30 4 1,15 * *` | 4:30 AM on 1st and 15th |

### 3.2 One-Time Scheduled Jobs

Execute once at a specific time.

```yaml
schedule:
  type: once
  at: "2026-01-22T14:30:00Z"
  # After execution, job moves to history
  delete_after_run: true
```

### 3.3 Interval-Based (Every N Minutes/Hours)

Simple interval scheduling.

```yaml
schedule:
  type: interval
  every: 15m          # or: 2h, 30s, 1d
  jitter: 30s         # Random offset to prevent thundering herd
  skip_if_running: true
```

### 3.4 Event-Based Triggers

React to system events (Phase 3).

```yaml
schedule:
  type: event
  trigger:
    type: file_created
    path: "/code/incoming/*"
  debounce: 5s
```

**Supported Event Types (Phase 3):**

| Event | Description |
|-------|-------------|
| `file_created` | New file in watched directory |
| `session_created` | New tmux session appears |
| `session_exited` | tmux session closes |
| `mail_received` | Incoming Gastown mail |
| `bead_status_change` | Issue status transition |
| `agent_idle` | Agent detects idle state |

### 3.5 Time Wheel Implementation

Internal scheduling uses a time wheel for efficient job dispatch:

```
┌───────────────────────────────────────────────────────────────┐
│                     Time Wheel (60 slots)                      │
│                                                                │
│    Now: slot 15                                                │
│    ▼                                                           │
│  ┌─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┬─┐│
│  │0│1│2│ │ │ │ │ │ │ │ │ │ │ │ │●│ │ │J│ │ │ │ │ │J│ │ │ │ │ ││
│  └─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┴─┘│
│    ▲                           ▲       ▲                       │
│    │                           │       │                       │
│    +- Minute 0                 │       +- Job at minute 24     │
│                                +- Job at minute 18             │
│                                                                │
│  Tick every second → Check current slot → Execute jobs        │
└───────────────────────────────────────────────────────────────┘
```

---

## 4. UI Design

### 4.1 Scheduler Tab Layout

```
┌─────────────────────────────────────────────────────────────────────┐
│ Terminal 1 │ Terminal 2 │ Files │ Beads │ Scheduler │ Settings │    │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  [+ New Job]  [Import]  [Export]                    [🔄 Refresh]    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─ Jobs ─────────────────────────────────────────────────────────┐ │
│  │                                                                 │ │
│  │  ┌──────────────────────────────────────────────────────────┐  │ │
│  │  │ ✓  Morning News Digest              ⏰ 0 8 * * * │ [Edit] │  │ │
│  │  │    🔵 Last: 2026-01-21 08:00 (✓ OK) │ Next: 08:00 │ [▶/⏸] │  │ │
│  │  └──────────────────────────────────────────────────────────┘  │ │
│  │                                                                 │ │
│  │  ┌──────────────────────────────────────────────────────────┐  │ │
│  │  │ ✓  Agent Health Check               ⏱ every 15m  │ [Edit] │  │ │
│  │  │    🟢 Last: 2026-01-21 09:45 (✓ OK) │ Next: 10:00 │ [▶/⏸] │  │ │
│  │  └──────────────────────────────────────────────────────────┘  │ │
│  │                                                                 │ │
│  │  ┌──────────────────────────────────────────────────────────┐  │ │
│  │  │ ✗  Backup Archives                  ⏰ 0 2 * * * │ [Edit] │  │ │
│  │  │    🔴 Last: 2026-01-21 02:00 (✗ FAIL)│ Next: --   │ [▶/⏸] │  │ │
│  │  │    ⚠ Error: Command timed out                            │  │ │
│  │  └──────────────────────────────────────────────────────────┘  │ │
│  │                                                                 │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌─ History ──────────────────────────────────────────────────────┐ │
│  │ Time         │ Job                  │ Status │ Duration │ Log  │ │
│  │──────────────┼──────────────────────┼────────┼──────────┼──────│ │
│  │ 09:45:00     │ Agent Health Check   │ ✓ OK   │ 1.2s     │ [📋] │ │
│  │ 09:30:00     │ Agent Health Check   │ ✓ OK   │ 1.1s     │ [📋] │ │
│  │ 09:15:00     │ Agent Health Check   │ ✓ OK   │ 1.0s     │ [📋] │ │
│  │ 08:00:00     │ Morning News Digest  │ ✓ OK   │ 45s      │ [📋] │ │
│  │ 02:00:00     │ Backup Archives      │ ✗ FAIL │ 5m00s    │ [📋] │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.2 Job Create/Edit Form

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Create Scheduled Job                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Name:  [Morning News Digest________________________]               │
│                                                                      │
│  ─── Schedule ───────────────────────────────────────────────────── │
│                                                                      │
│  Type:  (●) Cron  ( ) Interval  ( ) Once  ( ) Event                 │
│                                                                      │
│  Cron Expression: [0 8 * * *___________]                            │
│  ┌─ Preview: Daily at 8:00 AM ────────────────────────────────────┐ │
│  │  Next 5 runs:                                                   │ │
│  │    2026-01-22 08:00:00                                          │ │
│  │    2026-01-23 08:00:00                                          │ │
│  │    2026-01-24 08:00:00                                          │ │
│  │    2026-01-25 08:00:00                                          │ │
│  │    2026-01-26 08:00:00                                          │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  Timezone: [America/Los_Angeles      ▼]                             │
│                                                                      │
│  ─── Action ─────────────────────────────────────────────────────── │
│                                                                      │
│  Type:  ( ) Bash  (●) Prompt  ( ) Mail  ( ) Nudge  ( ) Health      │
│                                                                      │
│  Target Session: [hq-main              ▼]                           │
│    OR  Prefix:   [_____________________]  (e.g., gt-*)              │
│    OR  Group:    [HQ                   ▼]                           │
│                                                                      │
│  Prompt Content:                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ Please compile today's morning digest:                          ││
│  │ 1. Check unread mail (gt mail inbox)                            ││
│  │ 2. Summarize any overnight agent activity                       ││
│  │ 3. Highlight blocked or failed tasks                            ││
│  └─────────────────────────────────────────────────────────────────┘│
│                                                                      │
│  ─── Options ────────────────────────────────────────────────────── │
│                                                                      │
│  [✓] Enabled                                                        │
│  [ ] Skip if previous run still active                              │
│  Timeout: [5m_____]                                                 │
│  Retry on failure: [0] times, delay: [1m____]                       │
│                                                                      │
│                            [Cancel]  [Save Job]                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.3 Log Viewer Modal

```
┌─────────────────────────────────────────────────────────────────────┐
│  Job: Morning News Digest                              [✕]          │
│  Run: 2026-01-21 08:00:00 (ID: run-1705827600)                      │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Status: ✓ Completed                                                 │
│  Duration: 45.2s                                                     │
│  Target: hq-main                                                     │
│                                                                      │
│  ─── Execution Log ─────────────────────────────────────────────── │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ [08:00:00.000] Job triggered by scheduler                       ││
│  │ [08:00:00.050] Resolving target session: hq-main                ││
│  │ [08:00:00.102] Session found, sending prompt...                 ││
│  │ [08:00:00.150] tmux send-keys executed successfully             ││
│  │ [08:00:45.200] Prompt delivery confirmed                        ││
│  │ [08:00:45.200] Job completed                                    ││
│  └─────────────────────────────────────────────────────────────────┘│
│                                                                      │
│  ─── Prompt Sent ───────────────────────────────────────────────── │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ Please compile today's morning digest:                          ││
│  │ 1. Check unread mail (gt mail inbox)                            ││
│  │ 2. Summarize any overnight agent activity                       ││
│  │ 3. Highlight blocked or failed tasks                            ││
│  └─────────────────────────────────────────────────────────────────┘│
│                                                                      │
│                            [Re-run Now]  [Close]                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.4 UI Wireframe Summary

| Component | Purpose |
|-----------|---------|
| Job List | Compact view of all jobs with status indicators |
| Status Badge | Green (OK), Yellow (running), Red (failed), Gray (disabled) |
| Quick Actions | Enable/disable toggle, manual run, edit, delete |
| History Panel | Recent execution log with expandable details |
| Cron Preview | Human-readable description + next run times |
| Log Viewer | Full execution details in modal |

---

## 5. Use Cases

### 5.1 Morning News Digest Engine

**Goal:** Start each day with an AI-compiled summary of overnight activity.

```yaml
jobs:
  - id: morning-digest
    name: Morning News Digest
    schedule:
      type: cron
      expression: "0 8 * * 1-5"   # Weekdays 8:00 AM
    action:
      type: prompt
      target:
        session: hq-main
      content: |
        Good morning! Please compile today's digest:

        1. **Overnight Activity**
           - Run: gt trail --since yesterday
           - Summarize key accomplishments

        2. **Pending Work**
           - Run: gt ready
           - Highlight priority items

        3. **Issues**
           - Check for blocked or failed beads
           - Note any escalations

        4. **Agent Status**
           - Query witnesses for polecat health
           - Report any sessions requiring attention

        Present as a concise report.
```

### 5.2 Agentic Loop Robustness (Scheduled Nudges)

**Goal:** Keep agents active and prevent stalling.

```yaml
jobs:
  - id: idle-nudge
    name: Anti-Stall Nudge
    schedule:
      type: interval
      every: 30m
      jitter: 5m
    action:
      type: mail
      target:
        role: witness
      subject: "Idle Check"
      body: |
        Check all polecats for idle status.
        For any idle > 10 minutes with work on hook:
        - Send nudge: "You have work on your hook. GUPP applies."
        Report summary.

  - id: heartbeat
    name: Worker Heartbeat
    schedule:
      type: interval
      every: 5m
    action:
      type: health_check
      checks:
        - tmux_sessions_responsive
        - api_latency
```

### 5.3 Automated Backups

**Goal:** Regular backups of critical data.

```yaml
jobs:
  - id: backup-beads
    name: Backup Beads Database
    schedule:
      type: cron
      expression: "0 3 * * *"   # 3:00 AM daily
    action:
      type: bash
      target:
        session: shell1
      command: |
        cd /code && \
        tar -czf /vault/backups/beads-$(date +%Y%m%d).tar.gz .beads/ && \
        find /vault/backups -name 'beads-*.tar.gz' -mtime +7 -delete
      timeout: 10m

  - id: backup-configs
    name: Backup Configurations
    schedule:
      type: cron
      expression: "0 4 * * 0"   # Sunday 4:00 AM
    action:
      type: bash
      target:
        session: shell1
      command: |
        cp -r ~/.chrote /vault/backups/chrote-config-$(date +%Y%m%d)/
```

### 5.4 Health Monitoring Pings

**Goal:** Continuous system health monitoring.

```yaml
jobs:
  - id: health-monitor
    name: System Health Monitor
    schedule:
      type: interval
      every: 5m
    action:
      type: health_check
      checks:
        - api_health
        - ttyd_reachable
        - tmux_running
        - disk_space
      thresholds:
        disk_warning: 80%
        disk_critical: 95%
      notify_on_failure:
        - mail: --human
          subject: "CHROTE Health Alert"
        - toast: true

  - id: agent-health
    name: Agent Health Survey
    schedule:
      type: interval
      every: 15m
    action:
      type: mail
      target:
        broadcast: true
        role: witness
      subject: "Status Report"
      body: "Report polecat status: active, idle, or failed."
```

### 5.5 Use Case Summary

| Use Case | Schedule | Action Type | Purpose |
|----------|----------|-------------|---------|
| Morning Digest | Cron (8 AM) | Prompt | Daily briefing |
| Anti-Stall | Interval (30m) | Mail | Prevent idle agents |
| Heartbeat | Interval (5m) | Health Check | Monitor responsiveness |
| Beads Backup | Cron (3 AM) | Bash | Data protection |
| Config Backup | Cron (weekly) | Bash | Configuration safety |
| System Health | Interval (5m) | Health Check | Infrastructure monitoring |
| Agent Survey | Interval (15m) | Mail (broadcast) | Workforce visibility |

---

## 6. Error Handling

### 6.1 Failed Job Retry Logic

```yaml
# Per-job retry configuration
retry:
  enabled: true
  max_attempts: 3
  delay: 1m                    # Initial delay
  backoff: exponential         # linear | exponential | fixed
  max_delay: 15m              # Cap for exponential backoff

  # Retry conditions
  retry_on:
    - timeout
    - connection_error
    - session_not_found        # Target session missing

  # Don't retry on
  no_retry_on:
    - invalid_config
    - permission_denied
```

**Retry Flow:**
```
Attempt 1 → Fail → Wait 1m →
Attempt 2 → Fail → Wait 2m →
Attempt 3 → Fail → Mark as FAILED → Notify
```

### 6.2 Alert Notifications

```yaml
notifications:
  # Global defaults
  defaults:
    on_failure: true
    on_timeout: true
    on_success: false

  channels:
    - type: mail
      target: --human
      throttle: 5m              # Don't spam

    - type: toast
      enabled: true
      duration: 5s

    - type: mail
      target: town/deacon       # Escalate to Deacon
      only_on: critical
```

### 6.3 Logging and Auditing

**Log Structure:**
```
/home/chrote/.chrote/scheduler/
├── jobs.yaml                   # Job definitions
├── history/
│   ├── 2026-01-21.jsonl       # Daily execution log
│   └── 2026-01-20.jsonl
└── logs/
    └── morning-digest/
        ├── run-1705827600.log  # Detailed execution log
        └── run-1705741200.log
```

**History Entry (JSONL):**
```json
{
  "id": "run-1705827600",
  "job_id": "morning-digest",
  "triggered_at": "2026-01-21T08:00:00Z",
  "completed_at": "2026-01-21T08:00:45.2Z",
  "status": "success",
  "duration_ms": 45200,
  "target": "hq-main",
  "attempts": 1,
  "error": null
}
```

**Failed Run Entry:**
```json
{
  "id": "run-1705752000",
  "job_id": "backup-archives",
  "triggered_at": "2026-01-21T02:00:00Z",
  "completed_at": "2026-01-21T02:05:00Z",
  "status": "failed",
  "duration_ms": 300000,
  "target": "shell1",
  "attempts": 3,
  "error": {
    "type": "timeout",
    "message": "Command exceeded 5m timeout",
    "last_output": "Compressing files..."
  }
}
```

### 6.4 Error States

| State | Description | UI Indicator | Action |
|-------|-------------|--------------|--------|
| `success` | Job completed normally | Green checkmark | None |
| `failed` | All retries exhausted | Red X | Show error, notify |
| `timeout` | Exceeded timeout limit | Orange clock | Retry or fail |
| `skipped` | Previous run still active | Gray skip | Log reason |
| `disabled` | Job manually disabled | Gray circle | None |
| `paused` | Temporarily paused | Yellow pause | None |

---

## 7. Implementation Plan

### Phase 1: Core Scheduler Daemon

**Duration: Sprint 1**

**Goals:**
- Standalone scheduler daemon process
- Basic job execution (bash, prompt)
- YAML-based job storage
- Simple execution logging

**Deliverables:**

1. **Scheduler Daemon (`chrote-scheduler`)**
   - Go binary running as systemd service
   - Time wheel for efficient scheduling
   - Job store (YAML read/write)
   - HTTP API for job management

2. **Core Job Types**
   - `bash`: Execute commands in tmux sessions
   - `prompt`: Send text to tmux sessions

3. **Persistence**
   - `/home/chrote/.chrote/scheduler/jobs.yaml`
   - Basic execution history (JSONL)

4. **CLI Integration**
   ```bash
   gt sched list           # List jobs
   gt sched show <id>      # Show job details
   gt sched run <id>       # Manual trigger
   gt sched enable <id>    # Enable job
   gt sched disable <id>   # Disable job
   gt sched history        # Recent runs
   ```

5. **systemd Service**
   ```ini
   [Unit]
   Description=CHROTE Scheduler Daemon
   After=chrote-server.service

   [Service]
   Type=simple
   User=chrote
   ExecStart=/home/chrote/bin/chrote-scheduler
   Restart=always
   RestartSec=5

   [Install]
   WantedBy=multi-user.target
   ```

**Milestones:**
- [ ] Scheduler daemon boots and runs
- [ ] Cron parsing works correctly
- [ ] Jobs execute on schedule
- [ ] Logs persist to disk
- [ ] CLI shows job status

---

### Phase 2: UI Components

**Duration: Sprint 2**

**Goals:**
- Scheduler tab in dashboard
- Job create/edit forms
- History and log viewer
- Real-time status updates

**Deliverables:**

1. **Scheduler Tab**
   - Job list with status indicators
   - Enable/disable toggles
   - Manual run buttons
   - History panel

2. **Job Form**
   - Schedule type selector (cron/interval/once)
   - Cron expression builder with preview
   - Action type configuration
   - Target session/group selector

3. **History View**
   - Recent runs table
   - Status filtering
   - Log viewer modal
   - Re-run capability

4. **API Endpoints**
   ```
   GET    /api/scheduler/jobs          # List all jobs
   GET    /api/scheduler/jobs/:id      # Get job details
   POST   /api/scheduler/jobs          # Create job
   PATCH  /api/scheduler/jobs/:id      # Update job
   DELETE /api/scheduler/jobs/:id      # Delete job
   POST   /api/scheduler/jobs/:id/run  # Manual trigger
   GET    /api/scheduler/history       # Execution history
   GET    /api/scheduler/history/:id   # Run details with log
   ```

5. **React Components**
   - `<SchedulerTab />` - Main container
   - `<JobList />` - Job cards
   - `<JobCard />` - Individual job display
   - `<JobForm />` - Create/edit form
   - `<CronBuilder />` - Visual cron editor
   - `<HistoryPanel />` - Recent runs
   - `<LogViewer />` - Execution log modal

**Milestones:**
- [ ] Scheduler tab renders
- [ ] Jobs can be created via UI
- [ ] Jobs can be edited/deleted
- [ ] History displays correctly
- [ ] Logs are viewable

---

### Phase 3: Advanced Features

**Duration: Sprint 3**

**Goals:**
- Mail and nudge actions
- Health check job type
- Event-based triggers
- Enhanced error handling

**Deliverables:**

1. **Additional Job Types**
   - `mail`: Gastown mail integration
   - `nudge`: Direct agent nudges
   - `health_check`: Built-in health checks
   - `chain`: Sequential job execution

2. **Event-Based Scheduling**
   - File system watchers
   - Session event listeners
   - Mail arrival triggers

3. **Advanced Error Handling**
   - Configurable retry logic
   - Exponential backoff
   - Alert notifications
   - Throttling

4. **Enhancements**
   - Job import/export
   - Job templates library
   - Batch operations
   - Job dependencies (DAG)

5. **Monitoring**
   - Scheduler health endpoint
   - Prometheus metrics (optional)
   - Dashboard status widget

**Milestones:**
- [ ] Mail actions work
- [ ] Health checks execute
- [ ] Event triggers fire
- [ ] Retries work correctly
- [ ] Notifications delivered

---

### Milestone Summary

| Phase | Focus | Key Deliverables |
|-------|-------|------------------|
| **Phase 1** | Core Engine | Daemon, bash/prompt jobs, YAML store, CLI |
| **Phase 2** | UI | Dashboard tab, job forms, history, logs |
| **Phase 3** | Advanced | Mail/nudge, health checks, events, retries |

---

## 8. API Endpoints

### 8.1 Jobs API

```
# List all jobs
GET /api/scheduler/jobs
Response: {
  "jobs": [
    {
      "id": "morning-digest",
      "name": "Morning News Digest",
      "enabled": true,
      "schedule": { "type": "cron", "expression": "0 8 * * *" },
      "action": { "type": "prompt", ... },
      "last_run": { "at": "2026-01-21T08:00:00Z", "status": "success" },
      "next_run": "2026-01-22T08:00:00Z"
    },
    ...
  ]
}

# Get job details
GET /api/scheduler/jobs/:id
Response: {
  "id": "morning-digest",
  "name": "Morning News Digest",
  "enabled": true,
  "schedule": { ... },
  "action": { ... },
  "retry": { ... },
  "created_at": "2026-01-20T10:00:00Z",
  "updated_at": "2026-01-21T08:00:00Z",
  "stats": {
    "total_runs": 10,
    "success_count": 9,
    "failure_count": 1,
    "avg_duration_ms": 45000
  }
}

# Create job
POST /api/scheduler/jobs
Body: {
  "name": "New Job",
  "schedule": { "type": "cron", "expression": "0 * * * *" },
  "action": { "type": "bash", "target": { "session": "shell1" }, "command": "echo hello" }
}
Response: { "id": "new-job", ... }

# Update job
PATCH /api/scheduler/jobs/:id
Body: { "enabled": false }
Response: { "id": "new-job", "enabled": false, ... }

# Delete job
DELETE /api/scheduler/jobs/:id
Response: { "deleted": true }

# Manual trigger
POST /api/scheduler/jobs/:id/run
Response: {
  "run_id": "run-1705827600",
  "status": "started",
  "triggered_at": "2026-01-21T10:00:00Z"
}
```

### 8.2 History API

```
# List recent runs
GET /api/scheduler/history?limit=50&job_id=morning-digest&status=failed
Response: {
  "runs": [
    {
      "id": "run-1705827600",
      "job_id": "morning-digest",
      "job_name": "Morning News Digest",
      "triggered_at": "2026-01-21T08:00:00Z",
      "completed_at": "2026-01-21T08:00:45Z",
      "status": "success",
      "duration_ms": 45200
    },
    ...
  ],
  "total": 150,
  "has_more": true
}

# Get run details with log
GET /api/scheduler/history/:run_id
Response: {
  "id": "run-1705827600",
  "job_id": "morning-digest",
  "job_name": "Morning News Digest",
  "triggered_at": "2026-01-21T08:00:00Z",
  "completed_at": "2026-01-21T08:00:45Z",
  "status": "success",
  "duration_ms": 45200,
  "attempts": 1,
  "target": "hq-main",
  "action_sent": "Please compile today's morning digest...",
  "log": [
    { "time": "08:00:00.000", "level": "info", "message": "Job triggered by scheduler" },
    { "time": "08:00:00.050", "level": "info", "message": "Resolving target session: hq-main" },
    ...
  ],
  "error": null
}
```

### 8.3 Health API

```
# Scheduler health
GET /api/scheduler/health
Response: {
  "status": "ok",
  "uptime_seconds": 86400,
  "jobs_active": 5,
  "jobs_disabled": 2,
  "runs_today": 45,
  "failures_today": 1,
  "next_run": {
    "job_id": "health-monitor",
    "at": "2026-01-21T10:05:00Z"
  }
}
```

### 8.4 API Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/scheduler/jobs` | List all jobs |
| GET | `/api/scheduler/jobs/:id` | Get job details |
| POST | `/api/scheduler/jobs` | Create new job |
| PATCH | `/api/scheduler/jobs/:id` | Update job |
| DELETE | `/api/scheduler/jobs/:id` | Delete job |
| POST | `/api/scheduler/jobs/:id/run` | Trigger manual run |
| GET | `/api/scheduler/history` | List execution history |
| GET | `/api/scheduler/history/:run_id` | Get run details |
| GET | `/api/scheduler/health` | Scheduler status |

---

## 9. Security Considerations

### 9.1 Command Injection Prevention

All bash commands are validated and sanitized:

```go
// Validate command before execution
func validateCommand(cmd string) error {
    // Block shell metacharacters that could escape
    dangerous := []string{
        "$(", "`",           // Command substitution
        "&&", "||", ";",     // Command chaining (allow if intended)
        "|",                 // Piping (allow if intended)
        ">", ">>", "<",      // Redirection (allow if intended)
        "\n", "\r",          // Newlines
    }

    // Log warning for potentially dangerous patterns
    for _, pattern := range dangerous {
        if strings.Contains(cmd, pattern) {
            log.Warn("Command contains shell metacharacter",
                "pattern", pattern, "command", cmd)
        }
    }

    return nil
}
```

### 9.2 Target Session Validation

Jobs can only target existing tmux sessions:

```go
func validateTarget(target Target) error {
    if target.Session != "" {
        // Session must exist
        sessions, err := tmux.ListSessions()
        if err != nil {
            return err
        }

        found := false
        for _, s := range sessions {
            if s.Name == target.Session {
                found = true
                break
            }
        }

        if !found {
            return fmt.Errorf("session not found: %s", target.Session)
        }
    }

    return nil
}
```

### 9.3 Access Control

Scheduler routes use the same trust boundary as the rest of CHROTE: there is no
application login or access token. Bind CHROTE to localhost and expose it only
through an explicitly trusted private network such as a Tailscale tailnet. Job
target validation, command validation, and Unix-user permissions remain the
operation-level safeguards.

### 9.4 Audit Logging

All job operations are logged:

```go
type AuditEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Action    string    `json:"action"`    // create, update, delete, run
    JobID     string    `json:"job_id"`
    Actor     string    `json:"actor"`     // "api", "scheduler", "cli"
    Details   any       `json:"details"`
}

func auditLog(action, jobID, actor string, details any) {
    entry := AuditEntry{
        Timestamp: time.Now(),
        Action:    action,
        JobID:     jobID,
        Actor:     actor,
        Details:   details,
    }

    // Append to audit log
    appendToAuditLog(entry)
}
```

### 9.5 Resource Limits

Prevent resource exhaustion:

```yaml
# Global limits
limits:
  max_concurrent_jobs: 5          # Parallel execution limit
  max_job_duration: 30m           # Hard timeout
  max_jobs_per_minute: 10         # Rate limiting
  max_log_size_mb: 10             # Per-run log size
  max_history_days: 30            # History retention
```

### 9.6 Sensitive Data

Prompts and commands may contain sensitive data:

```yaml
# Jobs can reference secrets
action:
  type: bash
  command: "curl -H 'Authorization: Bearer ${API_TOKEN}' ..."
  env:
    API_TOKEN:
      source: env              # Read from environment
      # or:
      source: file
      path: /home/chrote/.secrets/api-token
```

**Secrets are never logged in plaintext:**
```go
func redactSecrets(log string, secrets []string) string {
    for _, secret := range secrets {
        log = strings.ReplaceAll(log, secret, "[REDACTED]")
    }
    return log
}
```

### 9.7 Security Summary

| Aspect | Implementation |
|--------|----------------|
| **Authentication** | None; host and private-network access are the trust boundary |
| **Authorization** | All jobs run as `chrote` user |
| **Injection Prevention** | Command validation and logging |
| **Target Validation** | Only existing sessions allowed |
| **Audit Trail** | All operations logged |
| **Resource Limits** | Concurrent jobs, timeouts, rate limits |
| **Secret Handling** | Environment/file references, redaction |
| **Log Retention** | Configurable, auto-cleanup |

---

## Acceptance Criteria

### Architecture
- [x] Architecture diagram included
- [x] Custom scheduler vs systemd decision documented
- [x] Integration with tmux sessions explained
- [x] Persistence layer defined

### Job Types
- [x] Bash execution documented
- [x] Prompt delivery documented
- [x] Mail-based triggers documented
- [x] Nudge support documented
- [x] Health check jobs documented

### Scheduling
- [x] Cron syntax support defined
- [x] Interval scheduling defined
- [x] One-time jobs defined
- [x] Event-based triggers defined (Phase 3)

### UI Design
- [x] Job list wireframe included
- [x] Create/edit form wireframe included
- [x] History panel wireframe included
- [x] Log viewer wireframe included

### API
- [x] All endpoints defined
- [x] Request/response formats documented
- [x] Error responses documented

### Error Handling
- [x] Retry logic defined
- [x] Notification channels defined
- [x] Logging structure defined
- [x] Error states enumerated

### Security
- [x] Command injection prevention addressed
- [x] Access control documented
- [x] Audit logging defined
- [x] Resource limits defined

### Implementation Plan
- [x] Phase 1 milestones defined
- [x] Phase 2 milestones defined
- [x] Phase 3 milestones defined
- [x] Clear deliverables per phase
