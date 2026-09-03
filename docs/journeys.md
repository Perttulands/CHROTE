# CHROTE operator journeys

Seven journeys describe everything the operator does in CHROTE. They are the
product's use cases: a control, a panel or a tab earns its place by serving a
step in one of them. [`DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md) derives its
doctrine from this document; [`PRD.md`](../PRD.md) cites it.

Every journey ends in one of two actions. Either the operator prompts an agent
through Send to Session, the product's one hand-off path, or he makes a small
manual edit himself. In the operator's words: "In the end CHROTE is
fundamentally about running agents and everything serves to make that as easy as
possible."

Chords are written as `Alt` and a key: `Alt` is the CHROTE key, and the chord
works whether or not a terminal has the cursor. The leader,
`Ctrl+Shift+Space`, is discovery — it opens a strip listing the same chords, and
inside its window the bare key runs the action. Where a journey names a surface
or a chord CHROTE does not have yet, the entry says so. Beads owns when it
arrives.

## 1. Launch an agent

**Goal.** Start a harness in the right folder, as the right Unix user, bound to
the window in front of the operator.

**Steps.**

1. Pick the window the agent should live in, or open the launcher from the
   Sessions panel.
2. Choose harness, folder, Unix user and name.
3. Adjust the flag line, which is prefilled with the harness's defaults; the
   preview shows the exact command that will run.
4. Launch. The session binds to the window it was launched from.

**Surfaces.** Terminal workspace, where an empty window is the launcher (1, 2,
3, 4); Sessions panel, whose plus opens the same launcher (1).

**End action.** The first prompt, typed into the tile, or carried in from
another surface through Send to Session and left for the operator to submit when
the agent is ready.

**Chords.** `Alt+N` opens the launcher in the focused window, or the panel
launcher when no window is empty. `Alt+W` steps to the next window.

## 2. Run and steer

**Goal.** Watch several agents at once and answer whichever one is asking.

**Steps.**

1. Arrange the work: each terminal tab is an independent layout of one to four
   windows.
2. Find the session by name, or by reading the tiles.
3. Focus the tile and type, or look at a session that is not on screen without
   taking it over.
4. Hand an object over instead of retyping it.

**Surfaces.** Tab bar (1); terminal workspace (1, 3); Sessions panel and its
search (2); Peek (3); the Send drawer (4).

**End action.** Typing in the tile, or Send to Session.

**Chords.** `Alt+1`–`Alt+6` for the terminal tabs that exist; `Alt+W` and
`Alt+Shift+W` for the next and previous window; `Alt+Plus` to add a window and
`Alt+Minus` to remove the last empty one; `Alt+A` to toggle the Sessions panel;
`Alt+P` to Peek at the focused tile's session; `Alt+S` to open the Send drawer.
Session search has no direct chord: leader then `/`.

## 3. Understand the work

**Goal.** Read what is planned, in progress and stale across projects, and hand
a Bead to an agent.

**Steps.**

1. Open Beads and choose the project store.
2. Read open work as a map and dig into it.
3. Read an epic, its goals and its definitions of done.
4. Find stale work to clear out.
5. Open a Bead id printed in terminal output and read it beside the session that
   named it.
6. Annotate the Bead and send it.

**Surfaces.** Beads tab (1, 2, 3, 4, 6); terminal workspace, for ids in output
(5); the Send drawer (6).

**End action.** Send to Session, carrying the Bead id and the note. Writing
Beads stays with `bd` and the agents.

**Chords.** `Alt+B` opens the Beads tab; `Alt+S` opens the Send drawer.

## 4. Understand what an agent sees

**Goal.** Know which instructions, skills and memories an agent loaded before
trusting or correcting it. The operator: "Understanding what the agents load
with and what they see is perhaps the most critical thing of all."

**Steps.**

1. Ask what a running agent sees: its instruction stack in order, the skills
   available to it, its memories, and where each came from. Not built yet.
2. Read the layer that surprised you.
3. Compare workspaces, when the same instruction should hold in several. Not
   built yet.
4. Correct the wording, or hand the curation to the agent that tends the layer.

**Surfaces.** A session's tile or row, as the way in (1); the instructions
surface (1, 2, 3); the editor shared with Files (4); the Send drawer (4).

**End action.** A small manual edit, or Send to Session.

**Chords.** None yet.

## 5. Review files and changes

**Goal.** Find a file, read it properly, see what changed, and either fix it by
hand or hand the path to an agent.

**Steps.**

1. Find the file by name across the configured roots, from the keyboard.
2. Read it beside the terminal: Markdown rendered in the theme, and images, JSON
   and code in viewers that suit them.
3. Compare it against its committed state.
4. Fix a small thing in place and save, or hand the path over.

**Surfaces.** The Files panel beside the terminal (1, 2, 3, 4); the Files tab,
for browsing (1, 2); the Send drawer (4).

**End action.** A small manual edit, or Send to Session with the file's path as
the reference.

**Chords.** `Alt+O` toggles the Files panel; `Alt+S` opens the Send drawer. Find
by name has no chord yet.

## 6. Watch the host

**Goal.** Notice that the host or CHROTE itself is degraded before it costs an
agent its work.

**Steps.**

1. Read health, resource readings and recent runtime events.
2. Confirm the reading against a terminal on the host.
3. Hand the failure to an agent, or change the setting that caused it.

**Surfaces.** Server tab (1); terminal workspace (2); Settings (3); the Send
drawer (3).

**End action.** Send to Session with what the reading says, or a small manual
edit in Settings.

**Chords.** None yet.

## 7. Keep the library

**Goal.** Understand what is in the context corpus, correct it where it is
wrong, and ask the librarian for the rest. The operator: "This library is an
opportunity to make something with real character and opinion instead of just a
filesystem."

**Steps.**

1. Step into the library: the map of its shelves and links, and pages rendered
   from their Markdown with their neighbours above them.
2. Search across everything, and read what changed recently.
3. Fix an odd wording where you find it; each save is a commit attributed to the
   operator.
4. Point the librarian at the page or the shelf.

**Surfaces.** The Library surface and its shelves, pages, search and recent
changes (1, 2, 3); the editor shared with Files (3); the librarian's own session
and the Send drawer (4).

**End action.** A small manual edit, or Send to Session pointed at the page.

**Chords.** `Alt+L` opens the Library tab; `Alt+S` sends the page on the table;
`Alt+R` turns the map over.
