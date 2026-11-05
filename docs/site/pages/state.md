---
title: Where state lives
nav: State and backups
summary: What CHROTE keeps, what it only shows, and what to back up.
sources: ARCHITECTURE.md, PRD.md, docs/installation.md
---

# Where state lives

CHROTE reads and controls the real thing instead of keeping its own copy. That
makes the backup question short.

| State | Lives in | Back it up? |
| --- | --- | --- |
| Sessions and their processes | tmux, in memory | Nothing to back up; they end with tmux or the host |
| Files | Your filesystem | Yes, as you already do |
| Issues and plans | Each project's `.beads` store | With the project |
| The Brain corpus | Its git repository | With git |
| Scheduled tasks and run history | `~/.local/state/chrote/scheduled-tasks/` | **Yes**, this is CHROTE's own state |
| Send to Session drops | `~/.local/state/chrote/session-drops/` | No; staging for delivery |
| Generated agent hook settings | `~/.local/state/chrote/agent-hooks/` | No; rewritten by the next launch |
| Your settings | `~/.config/chrote/secrets.env` | **Yes**; it may hold secrets, keep it private |
| Managed settings, the unit | `~/.config/chrote/chrote.env`, `~/.config/systemd/user/chrote.service` | No; the installer writes them |
| Launch configuration, theme, `services.json` | Wherever you keep operator configuration | **Yes** |
| The Beads tab's last reads | `~/.cache/chrote/beads-host/` | No; disposable |
| Health history, last agent event per session | CHROTE's memory | No; gone on restart by design |
| Layouts, tab labels, presets, device settings | This browser's `localStorage` | Optional; per device |

The XDG variables move the first three directories. Paths above are the
defaults.

## What survives what

| Event | Sessions | Files, Beads, schedules | Layout on this device |
| --- | --- | --- | --- |
| Close the browser | Kept | Kept | Kept |
| Clear browser storage | Kept | Kept | **Lost** |
| Restart CHROTE, upgrade | Kept | Kept | Kept |
| Re-run the installer | Kept | Kept | Kept |
| Uninstall | Kept | Kept, unless you purge | Kept |
| Kill the tmux server, reboot the host | **Lost** | Kept | Kept, bindings show ended sessions |

CHROTE does not recreate sessions after a reboot. A workload that must come back
needs its own systemd unit or equivalent, outside CHROTE.
