---
title: Server and Settings
nav: Server and Settings
summary: Watch the host before it costs an agent its work, and set what is yours to set on this device.
sources: PRD.md, docs/journeys.md, src/internal/api/system.go
---

# Server and Settings

## Server

The Server tab shows whether the host, and CHROTE itself, is degraded:

- **Host**: name, uptime, and load averages over 1, 5 and 15 minutes.
- **CPU**, **memory and swap**, **disks** and **network** throughput, and GPUs
  where the host reports them.
- **Warnings** the readings raise, and recent runtime events.
- **History**: the server samples on its own, whether or not a browser is open,
  and keeps a bounded history the tab charts.

Confirm a reading in a terminal before acting on it, then hand the failure to an
agent with Send to Session, or change the setting that caused it.

The same readings are at `GET /api/system/status` and `GET /api/system/history`;
the build is at `GET /api/health` and `GET /api/version`.

## Settings

Settings are per device unless they say otherwise; they live in this browser's
storage and never in tmux.

| Section | What it sets |
| --- | --- |
| Appearance | Terminal font size (12–20 px), mouse-wheel scrolling, hiding the terminal scrollbar |
| Session defaults | How often the session list refreshes; a name prefix per Unix user for new sessions; how many terminal tabs (1–6); which Unix user new sessions in each tab belong to |
| Agent events | Marks on rows and tabs, toasts, a tone per event kind, and browser notifications while the tab is hidden |
| Session cleanup | **Nuke all sessions**, an emergency action that kills every session on the configured tmux server after a second press |
| Beads projects | Extra store paths for discovery, for projects nested deeper than discovery looks |

There is no theme picker: the theme is host configuration, see
[Theme](theme.md). Keys on or off is the **Keys on** button at the right of the
tab bar.

> [!CAUTION]
> **Nuke all sessions** kills every session on the one configured user's tmux
> server, including ones CHROTE did not create. With several Unix users
> configured it refuses and kills nothing, because it does not name a user.
> Individual session controls are safer.

## Dashboard Help

The tab bar's overflow offers **Dashboard Help**, a short in-app guide. Its
keyboard section predates the Alt chords: it says dashboard shortcuts stay out
of terminal input and that actions use visible controls, which is no longer
how keys work. For keys, use the keybindings panel (`Alt+K`), which lists every
chord from the same registry that runs them, and [Keyboard](keyboard.md).
