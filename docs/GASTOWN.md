# Gastown Overview

CHROTE (**C**ontrol **H**ub for **R**emote **O**perations & **T**mux **E**xecution) is the infrastructure that powers **Gastown**, Steve Yegge's orchestration framework for running 10-30+ AI coding agents in parallel.

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Mayor** | AI coordinator with full workspace context; the human's primary interface |
| **Polecats** | Ephemeral worker agents that spawn, complete tasks, and disappear |
| **Rigs** | Project containers wrapping git repositories and managing agents |
| **Hooks** | Git worktree-based persistent storage; work survives agent crashes |
| **Beads** | Structured issue tracking with git-backed persistence |
| **Convoys** | Bundles of work (beads) assigned to agents |
| **Witness** | Monitors polecat workers and reports status |
| **Refinery** | Handles merges and branch management |

## The Mail System

Gastown uses a **mail-based messaging system** for agent communication:

```bash
gt mail inbox                              # Check messages
gt mail read <id>                          # Read specific message
gt mail send mayor/ -s "Subject" -m "Body" # Send to Mayor
gt mail send --human -s "..."              # Send to human overseer
gt mail send <rig>/witness -s "HELP" -m "..." # Escalate to witness
```

**Key insight**: Humans can send natural language messages to the Mayor, who then orchestrates agents. This "instant messenger to AI" pattern enables asynchronous human-agent collaboration.

## Session Naming Conventions

| Pattern | Role | Dashboard Group |
|---------|------|-----------------|
| `hq-*` | Headquarters coordination | HQ |
| `gt-{rig}-mayor` | Mayor for a rig | Rig group |
| `gt-{rig}-{N}` | Polecat workers (numbered) | Rig group |
| `gt-{rig}-witness` | Witness (monitors workers) | Rig group |
| `gt-{rig}-refinery` | Refinery (handles merges) | Rig group |

## Workflow

1. **Human talks to Mayor**: `gt mayor attach` or send mail
2. **Mayor creates convoy**: Bundles beads (issues) into work packages
3. **Polecats spawn**: Workers claim beads and execute tasks
4. **Witness monitors**: Reports progress, handles recovery
5. **Refinery merges**: Integrates completed work
6. **Mayor reports back**: Summarizes results to human

## GUPP Principle

**Gas Town Universal Propulsion Principle**: "If there is work on your hook, YOU MUST RUN IT."

All agents have persistent identities in Beads (git-backed). When an agent starts, it checks its hook for pending work and continues automatically. This enables:
- Work survives context window limits
- Agents can restart without losing state
- Asynchronous orchestration at scale

## References

- [Gastown GitHub](https://github.com/steveyegge/gastown)
- [Welcome to Gas Town (Medium)](https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04)
- [The Future of Coding Agents (Medium)](https://steve-yegge.medium.com/the-future-of-coding-agents-e9451a84207c)
