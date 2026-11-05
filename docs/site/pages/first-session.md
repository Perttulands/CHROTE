---
title: Your first session
nav: First session
summary: Launch a session into a window, watch a second one, look at a third without taking it over, and hand one of them work.
sources: docs/journeys.md, DESIGN-SYSTEM.md, README.md
---

# Your first session

This walk-through assumes a fresh install at <http://127.0.0.1:8094>. Nothing
in it can lose work: every step creates or shows sessions, and the only
destructive action, killing a session, asks twice.

## 1. Launch into the empty window

Terminal 1 opens with one empty window. **An empty window is the launcher.**
It asks four things:

- **Harness**: what to start. A fresh install offers `Shell`, your login shell.
  The host adds agent CLIs and their flags in [the launch
  configuration](launcher.md).
- **Folder**: where to start. Empty, the list shows pinned folders and the
  working directories of live sessions. Type a fragment and the host's
  workspaces are ranked by fuzzy match; type an absolute path and the
  directories under it are listed. `Tab` completes the highlighted row and
  `Enter` takes it.
- **Unix user**: whose tmux server the session lives on. With one configured
  user, there is nothing to choose.
- **Name**: derived from folder and harness, suffixed on collision, and
  editable.

Launch. The new session starts in that folder, with its harness running, and
binds to the window it was launched from. Click into the tile and type: a
focused tile sends every key to the program inside it.

> [!TIP]
> Already have tmux sessions? They are listed in the Sessions panel the moment
> CHROTE starts. Open a session row's menu and choose **Attach to window** to put
> it in the empty window instead.

## 2. Add a second window

Press `Alt+Plus` (`Alt+=` is the same chord on layouts where `+` needs Shift),
or open the terminal tab's menu and choose a window count. A tab shows one to
four windows. The new window is empty, so it is a launcher too: `Alt+N` puts
the launcher in the focused window.

Move between windows with `Alt+W` and `Alt+Shift+W`. `Alt+Minus` removes the
last empty window.

## 3. Find a session without moving anything

Open the Sessions panel with `Alt+A`. It lists sessions on every tmux socket
CHROTE reads, grouped and searchable.

- **Click a row** to Peek: a floating window that shows the session at its own
  size and takes your keys, so you can answer a prompt there. It never
  reassigns a window. `Escape` outside its terminal, or a click elsewhere,
  closes it.
- **The row's menu** (right-click, or long-press on touch) attaches the session
  to a window, opens Send to Session, schedules a task, shows **What this agent
  sees**, renames, copies or kills it.
- **A location chip** on a session already showing somewhere jumps to that
  window.

## 4. Hand work to an agent

Focus a tile and press `Alt+S`. The Send drawer slides in from the right with
the target already chosen: the focused tile's session. Write the note and press
**Send**, which pastes and submits. **Paste** puts it in the prompt without
submitting, for a line you want to read first.

The same drawer opens from a file, a diff, a Bead or a Brain page, carrying a
one-line reference to the object. See [Send to Session](send-to-session.md).

## 5. Let agents tell you when they are done

When you launch Claude Code or Codex from the launcher with **Notify on
completion** on (the default), the harness reports when it finishes a turn or
needs input. CHROTE marks the session's row and its tab, and names the session
in a toast. Under **Settings → Agent events** each device can also play a tone
or show a browser notification while the tab is hidden. Focusing the session
marks it seen on every device.

## 6. Pick it up from somewhere else

Close the browser. The sessions keep running in tmux. Open CHROTE on another
device and they are all there; the layout is per device, so arrange it again or
save it as a preset from the terminal tab's menu (**Save layout as preset**, up
to ten).

On a screen narrower than 768 pixels, the tab bar folds into a menu and one
window shows at a time. A phone is good for reading output, answering a prompt
and sending a message; arranging four windows wants a real screen. To reach
CHROTE from a phone at all, set up [remote access](remote-access.md).

## If keys do not do what you expect

`Alt` is the CHROTE key: a registered `Alt` chord runs a CHROTE action even
inside a focused terminal, and every other `Alt` combination reaches the
program untouched, as does `AltGr`. If a program you use needs one of CHROTE's
chords, the **Keys on** button at the right of the tab bar turns interception
off for this device. `Ctrl+Shift+Space` or `Alt+K` lists every chord. See
[Keyboard](keyboard.md).
