# CHROTE operator journeys

Seven journeys describe everything the operator does in CHROTE. They are the
product's use cases: a control, a panel or a tab earns its place by serving a
step in one of them. [`DESIGN-SYSTEM.md`](../DESIGN-SYSTEM.md) derives its
doctrine from this document; [`PRD.md`](../PRD.md) cites it.

Every journey ends in one of two actions. Either the operator prompts an agent
or he makes a small manual edit himself. The prompt goes through Send to
Session, the product's hand-off path, or, in a tab with a resident agent, is
pasted straight into the resident's prompt for him to finish and submit there.
In the operator's words: "In the end CHROTE is fundamentally about running
agents and everything serves to make that as easy as possible."

Chords are written as `Alt` and a key: `Alt` is the CHROTE key, and the chord
works whether or not a terminal has the cursor. The leader,
`Ctrl+Shift+Space`, is discovery: it opens the keybindings panel listing the
same chords, and inside its window the bare key runs the action. Where a
journey names a surface or a chord CHROTE does not have yet, the entry says so.
Beads owns when it arrives.

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
a Bead to the Clerk, the agent that keeps the register.

**Steps.**

1. Open Beads and choose the project store from the rail.
2. Read open work as a map. A click on a row puts its Bead on the table and,
   where the row has children, folds or unfolds them.
3. Read an epic, its goals and its definitions of done.
4. Read the same epic as a flow: waves of work from left to right, the Beads
   that can run at once stacked in one column, the blocking edges drawn
   between them. The arrow keys travel it wave by wave and down a column, and
   a click puts a Bead on the table and brings it to the middle.
5. Find stale work to clear out.
6. Open a Bead id printed in terminal output and read it on the table beside
   the session that named it.
7. Annotate the Bead and hand it to the Clerk, whose column is at the right of
   the tab.

**Surfaces.** Beads tab, its map, its flow and the table (1, 2, 3, 4, 5, 7);
terminal workspace with the table at its right, for ids in output (6); the
Clerk's column (7); the Send drawer, for any other session (7).

**End action.** Paste into the Clerk: `Alt+S` puts the Bead's reference into the
Clerk's prompt, and the operator writes the rest and presses `Enter` there.
Send to Session carries the same reference to any other session. Writing Beads
stays with `bd` and the agents, the Clerk among them.

**Chords.** `Alt+B` opens the Beads tab; `Alt+S` pastes the Bead on the table
into the Clerk's prompt; `Alt+Enter` focuses the Clerk's column; `Alt+I` closes
the table.

## 4. Understand what an agent sees

**Goal.** Know which instructions, skills and memories an agent loaded before
trusting or correcting it. The operator: "Understanding what the agents load
with and what they see is perhaps the most critical thing of all."

**Steps.**

1. Ask what a running agent sees: its instruction stack in order, the skills
   available to it, its memories, and where each came from.
2. Read the layer that surprised you.
3. Compare workspaces, when the same instruction should hold in several. Not
   built yet.
4. Correct the wording, or hand the curation to the tender, the agent that
   tends the layers, whose column is at the right of the Agents tab.

**Surfaces.** A session's tile or row, as the way in (1); What this agent sees,
on the table (1, 2); the Agents tab, for any workspace and harness (1, 2, 3);
the editor shared with Files (4); the tender's column (4).

**End action.** A small manual edit, or paste into the tender: `Alt+S` puts the
chosen workspace and harness into the tender's prompt. Send to Session carries
the same reference to any other session.

**Chords.** `Alt+G` opens the Agents tab; `Alt+S` pastes into the tender's
prompt; `Alt+Enter` focuses the tender's column.

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
wrong, and ask the Librarian for the rest. The operator: "This library is an
opportunity to make something with real character and opinion instead of just a
filesystem."

**Steps.**

1. Step into the library at its map: every page, shelves as labelled clusters,
   wiki links and shared tags as edges, size from length, brightness from
   recency, candidates dim. Click a page to dive into it: the map takes it to
   the middle and closer in, and the page opens in a column beside the map with
   its neighbours as links to travel by and a trail back through the dive.
2. Work the map from the rail: a shelf opens inside it rather than over the
   map, pointing at one of its pages takes the map to that page and lights it,
   and clicking the page opens the row on what it is with a dive to take.
3. Search across everything, and read what arrived lately: the pages that
   changed, newest first, worked the same way as a shelf's.
4. Fix an odd wording where you find it; each save is a commit attributed to the
   operator.
5. Point the Librarian at the page or the shelf.

**Surfaces.** The Library's map and the dive column beside it (1); the rail of
shelves and arrivals beside the map, with its search (2, 3); the editor shared
with Files (4); the Librarian's column (5).

**End action.** A small manual edit, or paste into the Librarian: `Alt+S` puts
the page being dived into into the Librarian's prompt, and a shelf's menu sends
the shelf the same way.

**Chords.** `Alt+L` opens the Library tab; `Escape` closes the dive and leaves
the map where it was; `Alt+S` pastes the page into the Librarian's prompt;
`Alt+Enter` focuses the Librarian's column.
