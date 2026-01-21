# Gas Town Characters and Roles

Gas Town is an orchestration framework for running multiple AI coding agents in parallel. This document describes each character role, what they do, when to use them, and how they interact.

## Overview

Gas Town uses a hierarchy of specialized agents, each with distinct responsibilities:

| Character | Scope | Lifecycle | Primary Function |
|-----------|-------|-----------|------------------|
| **Mayor** | Town-wide | Persistent | Human interface & coordination |
| **Deacon** | Town-wide | Persistent | System health & watchdog |
| **Witness** | Per-rig | Persistent | Polecat health monitoring |
| **Refinery** | Per-rig | Persistent | Merge queue processing |
| **Polecats** | Per-rig | Ephemeral | Feature work |
| **Crew** | Per-rig | Persistent | Human developer workspaces |
| **Dogs** | Cross-rig | Reusable | Infrastructure & cleanup |
| **Boot** | Town-wide | Per-tick | Daemon triage |

---

## Mayor

**Role**: Global coordinator and the human Overseer's Chief of Staff.

### What It Does

The Mayor is the primary interface between humans and the automated agent system:

- Receives and routes escalations from Witnesses and Deacon
- Coordinates work across multiple rigs
- Handles human communication and strategic decisions
- Routes cross-project issues
- Creates convoys (bundles of work) for distribution

### When to Use

- Talk to the Mayor when you want to give instructions to the system
- Send mail to the Mayor for async communication
- Attach to the Mayor session for interactive work

### Commands

```bash
gt mayor start       # Start the Mayor session
gt mayor attach      # Attach to the Mayor session
gt mayor status      # Check Mayor session status
gt mayor stop        # Stop the Mayor session
gt mayor restart     # Restart the Mayor session
```

### Mail Address

Use `mayor` or `mayor/` in mail commands to reach the Mayor.

---

## Deacon

**Role**: Town-level watchdog and health monitor.

### What It Does

The Deacon ("daemon beacon") is the only agent that receives mechanical heartbeats from the daemon. It monitors system health across all rigs:

- Watches all Witnesses (are they alive? stuck? responsive?)
- Manages Dogs for cross-rig infrastructure work
- Handles lifecycle requests (respawns, restarts)
- Receives heartbeat pokes and decides what needs attention
- Cleans up orphaned agent processes

### When to Use

The Deacon operates automatically in the background. You typically don't interact with it directly unless:

- You need to force-kill an unresponsive agent
- You want to check system-wide health
- You need to clean up orphaned processes

### Commands

```bash
gt deacon start              # Start the Deacon session
gt deacon attach             # Attach to the Deacon session
gt deacon status             # Check Deacon session status
gt deacon health-check       # Send health check ping to an agent
gt deacon health-state       # Show health check state for all agents
gt deacon cleanup-orphans    # Clean up orphaned claude subagent processes
gt deacon force-kill         # Force-kill an unresponsive agent session
gt deacon pause              # Pause the Deacon to prevent patrol actions
gt deacon resume             # Resume the Deacon to allow patrol actions
```

### Relationship with Other Characters

**The Deacon patrols the town; Witnesses patrol their rigs; Polecats work.**

---

## Witness

**Role**: Per-rig polecat health monitor.

### What It Does

The Witness patrols a single rig, watching over its polecats:

- Detects stalled polecats (crashed or stuck mid-work)
- Nudges unresponsive sessions back to life
- Cleans up zombie polecats (finished but failed to exit)
- Nukes sandboxes when polecats complete via `gt done`

### What It Does NOT Do

- Force session cycles or interrupt working polecats
- Manage session lifecycle (polecats manage their own sessions via `gt handoff`)

The Witness only handles failures and edge cases.

### When to Use

One Witness per rig. The Witness operates automatically. You interact with it when:

- You need to check polecat health status
- You want to escalate an issue
- You need to restart the Witness itself

### Commands

```bash
gt witness start     # Start the witness
gt witness attach    # Attach to witness session
gt witness status    # Show witness status
gt witness stop      # Stop the witness
gt witness restart   # Restart the witness
```

### Mail Address

Use `witness` or `<rig>/witness` in mail commands.

---

## Refinery

**Role**: Per-rig merge queue processor.

### What It Does

The Refinery serializes all merges to main for a rig:

- Receives MRs (merge requests) submitted by polecats via `gt done`
- Rebases work branches onto latest main
- Runs validation (tests, builds, checks)
- Merges to main when clear
- If conflict: spawns a FRESH polecat to re-implement (original is already gone)

### Work Flow

```
Polecat completes → gt done → MR in queue → Refinery processes → Merge to main
```

Note: The polecat is already nuked by the time the Refinery processes.

### When to Use

One Refinery per rig. It operates automatically. You interact with it when:

- You want to see the merge queue status
- You need to claim or release an MR
- You want to see blocked MRs

### Commands

```bash
gt refinery start        # Start the refinery
gt refinery attach       # Attach to refinery session
gt refinery status       # Check refinery status
gt refinery queue        # Show merge queue
gt refinery ready        # List MRs ready for processing
gt refinery blocked      # List MRs blocked by open tasks
gt refinery claim        # Claim an MR for processing
gt refinery release      # Release a claimed MR back to the queue
```

### Mail Address

Use `refinery` or `<rig>/refinery` in mail commands.

---

## Polecats

**Role**: Ephemeral workers that build features.

### What They Do

Polecats are the primary workers in Gas Town:

- Spawned for one task
- Complete the task
- Run `gt done` to submit work
- Get nuked (sandbox destroyed)

There is NO idle state. A polecat is either:

- **Working**: Actively doing assigned work
- **Stalled**: Session crashed mid-work (needs Witness intervention)
- **Zombie**: Finished but `gt done` failed (needs cleanup)

### Self-Cleaning Model

When work completes:
1. Polecat runs `gt done`
2. Branch is pushed
3. Work submitted to merge queue
4. Polecat exits
5. Witness nukes the sandbox

Polecats don't wait for more work.

### Session vs Sandbox

- **Session**: The Claude session cycles frequently (handoffs, compaction)
- **Sandbox**: The git worktree persists until nuke

Work survives session restarts.

### When to Use

Polecats are spawned automatically by the system when work needs to be done. You don't typically interact with them directly, but you can:

- Check on their work via the dashboard
- Peek at their output
- Nuke them if stuck

### Commands

```bash
gt polecat identity      # Manage polecat identities
gt polecat check-recovery # Check if polecat needs recovery vs safe to nuke
gt polecat git-state     # Show git state for pre-kill verification
gt polecat gc            # Garbage collect stale polecat branches
```

### Key Principle

**Cats build features. Dogs clean up messes.**

---

## Crew

**Role**: Persistent workspaces for human developers.

### What They Do

Crew workers are persistent workspaces, unlike ephemeral polecats:

- Full git clones (not worktrees)
- User-managed lifecycle
- Stay until you remove them
- Keep uncommitted changes around

### Features

- Gas Town integrated: Mail, nudge, handoff all work
- Recognizable names: dave, emma, fred (not ephemeral pool names)
- Tmux optional: Can work in terminal directly without tmux session

### Crew vs Polecats

| Aspect | Polecats | Crew |
|--------|----------|------|
| Lifecycle | Ephemeral | Persistent |
| Management | Witness-managed | User-managed |
| Cleanup | Auto-nuked after work | Stays until removed |
| Use case | Single tasks | Exploratory/long-running work |

### When to Use

Use crew workers for:

- Exploratory work
- Long-running tasks
- When you want to keep uncommitted changes
- Human developers working alongside agents

### Commands

```bash
gt crew add <name>       # Create workspace without starting
gt crew start <name>     # Start session (creates workspace if needed)
gt crew stop <name>      # Stop session(s)
gt crew at <name>        # Attach to session
gt crew list             # List workspaces with status
gt crew remove <name>    # Remove workspace
gt crew refresh <name>   # Context cycle with handoff mail
gt crew restart <name>   # Kill and restart session fresh
gt crew pristine <name>  # Sync crew workspaces with remote
gt crew rename <old> <new> # Rename a crew workspace
```

---

## Dogs

**Role**: Reusable workers for infrastructure and cleanup.

### What They Do

Dogs are managed by the Deacon for town-level work:

- Infrastructure tasks (rebuilding, syncing, migrations)
- Cleanup operations (orphan branches, stale files)
- Cross-rig work that spans multiple projects

### Dogs vs Polecats (Cats)

| Aspect | Polecats (Cats) | Dogs |
|--------|-----------------|------|
| Scope | One rig | Cross-rig |
| Lifecycle | Ephemeral (one task, nuked) | Reusable (multiple tasks, eventually recycled) |
| Purpose | Build features | Clean up messes |
| Management | Witness | Deacon |

### Key Principle

**Cats build features. Dogs clean up messes.**

### When to Use

Dogs are dispatched automatically by the Deacon. You don't typically spawn dogs manually.

### Commands

```bash
gt dog add <name>        # Create a new dog in the kennel
gt dog list              # List all dogs in the kennel
gt dog status            # Show detailed dog status
gt dog call              # Wake idle dog(s) for work
gt dog dispatch          # Dispatch plugin execution to a dog
gt dog remove <name>     # Remove dogs from the kennel
```

### Location

The kennel is at `~/gt/deacon/dogs/`. The Deacon dispatches work to dogs.

---

## Boot

**Role**: Special dog that runs daemon triage.

### What It Does

Boot is a special dog that runs fresh on each daemon tick:

1. Observes system state
2. Decides whether to start/wake/nudge/interrupt the Deacon
3. Cleans inbox (discards stale handoffs)
4. Exits (or handoffs in non-degraded mode)

This centralizes the "when to wake" decision in an agent that can reason about it.

### When to Use

Boot operates automatically on each daemon tick. You typically don't interact with it directly.

### Commands

```bash
gt boot spawn            # Spawn Boot for triage
gt boot status           # Show Boot status
gt boot triage           # Run triage directly (degraded mode)
```

### Location

`~/gt/deacon/dogs/boot/` with session `gt-boot`

---

## Character Interactions

### Hierarchy

```
Human Overseer
      │
      ▼
   Mayor ◄────────────────┐
      │                   │ Escalations
      ▼                   │
   Deacon ─────► Dogs     │
      │                   │
      ▼                   │
  Witnesses ──────────────┘
      │
      ▼
  Polecats ────► Refinery
      │              │
      ▼              ▼
   gt done      Merge to main
```

### Communication Flow

1. **Human → Mayor**: Natural language via mail or direct attach
2. **Mayor → Polecats**: Work distribution via sling/convoy
3. **Polecats → Refinery**: Work submission via `gt done`
4. **Witnesses → Mayor/Deacon**: Escalations for stuck/failed polecats
5. **Deacon → Dogs**: Infrastructure and cleanup tasks

### The GUPP Principle

**Gas Town Universal Propulsion Principle**: "If there is work on your hook, YOU MUST RUN IT."

All agents check their hook for pending work on startup. This enables:

- Work survives context window limits
- Agents can restart without losing state
- Asynchronous orchestration at scale

---

## Quick Reference

### Role Shortcuts in Mail

| Shortcut | Resolves To |
|----------|-------------|
| `mayor` | Town Mayor |
| `deacon` | Town Deacon |
| `witness` | Current rig's Witness |
| `refinery` | Current rig's Refinery |
| `<rig>/witness` | Specific rig's Witness |
| `<rig>/refinery` | Specific rig's Refinery |

### Session Naming

| Pattern | Role |
|---------|------|
| `gt-{rig}-mayor` | Mayor for a rig |
| `gt-{rig}-{N}` | Polecat workers (numbered) |
| `gt-{rig}-witness` | Witness |
| `gt-{rig}-refinery` | Refinery |
| `gt-boot` | Boot |

### Common Commands

```bash
# Check what's on your hook
gt hook

# See all roles
gt role list

# Check system status
gt status

# Send mail
gt mail send <recipient> -s "Subject" -m "Message"

# Check inbox
gt mail inbox
```
