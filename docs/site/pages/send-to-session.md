---
title: Send to Session
nav: Send to Session
summary: The one way to hand work to an agent, and what its receipt does and does not promise.
sources: DESIGN-SYSTEM.md, PRD.md, docs/journeys.md, docs/scheduled-tasks.md
---

# Send to Session

Every journey in CHROTE ends in one of two actions: you prompt an agent, or you
make a small edit yourself. Send to Session is how the prompt gets there without
retyping what you are looking at.

## Open it from the thing you are handing over

| From | The reference it carries |
| --- | --- |
| A focused tile (`Alt+S`), a session row, Peek | The session |
| A file or folder in Files | Its path |
| A diff | The file and its change |
| A Bead | Its id and title, and its store's folder when two stores share a prefix |
| A Brain page | The page |
| What this agent sees | The workspace and harness |
| An element in Dev mode | The component and the file it is written in |

## The drawer

The drawer slides in from the right edge over the table's column, 380 px wide on
desktop, so nothing beneath it moves.

1. **The reference** comes first, read-only: one line the agent can act on.
2. **The note** is where the cursor lands. Write what you want done.
3. **The target** defaults to the focused tile's session. Otherwise a searchable
   picker lists live sessions, and **new agent** opens the launcher carrying the
   message.
4. **Send** pastes and submits. **Paste** puts the text in the prompt without
   submitting, for a prompt you want to read before it runs.

On success the tile scrolls to the bottom and the drawer closes. On failure the
drawer stays open with the server's own message. A drawer holding typed text
asks in place before discarding it.

On a phone the drawer fills the visible screen, a target summary with **Change**
replaces the picker, and changing target keeps the draft. Return adds a line;
Send and Paste stay explicit buttons.

## Residents take the paste

In a tab with a resident agent (Beads, Brain, Agents, Services), `Alt+S` and an
object's Send put the table's reference into the resident's prompt **without
submitting**. You finish the sentence in the resident's drawer and press `Enter`
there. The drawer serves every other session.

## Files travel with it

Text and files sent this way are staged under the Send to Session drops
directory (`CHROTE_SESSION_DROPS_DIR`, under CHROTE's state directory on an
installed host). Each drop is readable only by the target Unix user, through a
POSIX ACL, which is why the installer requires `setfacl`.

## What the receipt means

Send reports delivery to tmux. CHROTE verified the target pane, pasted the text
while that pane was still the one it resolved, and dispatched the submit key. It
does not claim the program in that pane read, accepted or understood it. If the
pane changed in between, the send fails instead of landing somewhere else.

## Schedule it instead

**Schedule task…** on a session row opens the same drawer with that session
fixed as the target and a schedule beside the note. See
[Scheduled](scheduled.md).

## Report a problem from anywhere

`Alt+R` opens a small box for filing a bug or feature as a Bead: the first line
is the title, the rest the description. **File & send** (`Ctrl+Enter`) files it
and then opens the Send drawer with the new Bead, so an agent can pick it up
straight away.

**Dev mode** (`Alt+Shift+D`) is the same hand-off for the dashboard's own
faults: a highlight follows the pointer, a label names the component under it
and its file, and a click opens the drawer with that line already written.
