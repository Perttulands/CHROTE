---
title: Concepts
nav: Concepts
summary: The handful of ideas the rest of CHROTE is built from, and who owns each piece of state.
sources: README.md, PRD.md, ARCHITECTURE.md, DESIGN-SYSTEM.md, docs/adr/0017-terminal-viewing-model.md
---

# Concepts

## Sessions, windows and tabs

**A session** is a tmux session on one of the tmux sockets CHROTE is
configured to read. It belongs to tmux and to the Unix user whose socket it
lives on. CHROTE can create one, attach to one, rename one and, when you ask
for exactly that session, kill one. It never kills a session on its own.

**A terminal tab** (Terminal 1, Terminal 2 …) is an independent workspace. You
get three by default and can have one to six; Settings sets the count. Hiding a
tab never deletes its layout or its sessions.

**A window** is one tile in a tab. A tab shows one to four windows at a time. A
window can hold several sessions and displays one of them; you cycle through the
rest without losing the binding.

**A binding** is your intent that a session lives in a window. It is yours until
you remove it. If the session ends, or another client detaches your terminal,
the tile keeps its last output and offers the next move instead of quietly
showing something else.

## Looking without taking over

**Peek** is a floating window over the workspace that shows a session at its own
size, status line included. Clicking a session row opens Peek. It never
reassigns a window, because losing a layout to a stray click is worse than one
extra click. Peek takes your keys, so you can answer a prompt without opening
the session in a window. It shrinks its font to fit rather than resizing the
session.

**Claim** settles the size. tmux draws a window once, so everyone watching a
session sees the size one viewer set. Showing a session displaces nobody: a
second device, or a second person, sees it live alongside you. `Claim` makes the
session fit the device you are on, and the others keep watching at that size.
`Alt+C` claims every connected tile at once.

**The location chip** on an attached session jumps to the window already
showing it.

## The launcher

An empty window is the launcher. It offers four choices: the **harness** (the
program to start, such as an agent CLI or a plain shell), the **folder**, the
**Unix user** and the **name**. The harnesses, their flags and the suggested
folders are host configuration; on a fresh install the launcher offers `Shell`
in `~`. The new session starts in that folder, as that user, with its harness
running, and binds to the window it was launched from. See
[The launcher](launcher.md).

## Handing work over

**Send to Session** is the one way to hand work to an agent. Every object that
can be handed over opens it: a tile, a session row, Peek, a file, a diff, a Bead,
a Brain page. It carries a one-line reference to the object, you add a note, and
Send pastes and submits into the target session. The receipt means CHROTE
delivered keystrokes to tmux. It does not claim the program read them. See
[Send to Session](send-to-session.md).

**The table** holds the one object you have selected: a Bead, a page, what an
agent sees. It is global, so the object chosen in one tab is on the table in
every tab, shown as a resizable column at the right.

**A resident** is an agent that lives in a tab, as a tmux session behind a thin
spine at the tab's right edge: the Librarian in Brain, the tender in Agents, the
Clerk in Beads, and a service agent in Services. Opened, it is a drawer with a
live terminal. In a tab with a resident, `Alt+S` pastes the table's reference
straight into the resident's prompt for you to finish. Which session and folder
each resident uses is host configuration; without it, the drawer says the
resident is not configured. See [Libraries and residents](integrations.md).

## Agent events

A harness launched from CHROTE can report that it **finished** a turn or
**needs input**, through its own completion hook. CHROTE marks the session's row
and tab until you focus the session, and can show a toast, play a tone or send a
browser notification. CHROTE never guesses this from terminal output; Claude
Code and Codex are wired automatically, other harnesses are not. See
[Agent events](launcher.md#agent-events).

## Who owns what

| State | Owner | What CHROTE does |
| --- | --- | --- |
| Live sessions and their processes | tmux | Discover, create, attach, display, and control on explicit request |
| Files | The host filesystem | Read and write within configured roots, with Unix permissions |
| Issues and plans | Each project's Beads store | Read through `bd`; writing stays with `bd` and agents |
| Schedules, service catalog, theme, launcher | Host configuration | Serve them and act on them |
| Layouts, tab labels, presets, device preferences | The browser's `localStorage` | Render this device's workspace |
| The last agent event per session | CHROTE process memory | Show it until you look; forgotten with the session |

If CHROTE stops, sessions, files and Beads are where they were. If you clear
browser storage, you lose an arrangement, not work.

## What CHROTE deliberately does not do

- It has **no login** and no roles. The network is the boundary; see
  [Security](security.md).
- It does not **supervise** processes. It will not restart a process that died
  or rebuild your work after a reboot. Those jobs belong to tmux, your shell,
  or a systemd unit you wrote on purpose.
- It does not **sandbox** agents. File roots bound CHROTE's own file API; a
  program in tmux keeps its Unix user's permissions.
- It is not a multi-user collaboration tool or a hosted service. See the
  non-goals in [`PRD.md`](../../../PRD.md#non-goals).
