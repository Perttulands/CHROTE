---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Design System Spec

Status: **Active core source of truth**.

CHROTE is a private instrument for real work: legible, honest about state, and
a pleasure to use. Its character comes from the work itself, as the brain drawn
as a night sky does, never from ornament laid over it, and nothing competes
with the terminal.

## Design principles

1. **The content is the interface.** The operator acts on the thing itself (a
   star, a row, a legend entry), not on a panel or toolbar about it. Context
   menus, drag and drop and inline popovers beat modal workflows.
2. **One idea per view, with character that carries meaning.** Each surface
   draws its data one way, and its look says something true: brightness is
   importance, haze is evidence, dim is finished. Ornament that says nothing is
   cut.
3. **Quiet until asked.** At rest a surface shows the work. Names, counts and
   secondary controls appear where the operator looks, on hover, focus or
   selection, in space reserved for them.
4. **Motion explains change.** A state change glides from where things were, so
   the eye can follow what changed. Nothing snaps, nothing loops for show, and
   reduced motion gets the end state at once.
5. **Generous targets.** Nothing that acts is smaller than 28px in either
   direction, a row acts as a whole, and every control comes from the shared
   kit, never from the browser's defaults.
6. **Plain words, not storage.** Surfaces show titles, subjects and names.
   Paths, kinds and ids appear only where the id is the point.
7. **Reading gets room.** Prose is set a notch larger than the chrome, at a
   comfortable measure, with space between its paragraphs (see Typeface).
8. **Host-state first.** Make clear what is durable host state and what is
   browser preference.
9. **Failure visibility.** Empty, degraded, blocked, and errored states are
   explicit and actionable.
10. **Colour is information, and the accent is the operator's.** Every colour
    marks a meaning. The accent, violet by default, is the one that marks
    *you*: where you are, where input goes, what you chose, what is alive, and
    the one button that acts. Used for exactly that everywhere, it is what
    makes CHROTE one product with a character of its own.
11. **Recognition over labels.** A known agent shows its mark; anything else is
    shown in its own words, untranslated.
12. **Accessible contrast.** Themes may differ; state and text stay readable.

## Doctrine

The principles say what CHROTE is like. The doctrine decides cases. It is
derived from [`docs/journeys.md`](docs/journeys.md), which names the seven
journeys the product exists to serve, and it applies to any control on any
surface. Every control CHROTE carries was defensible when it was added; the sum
of them is the wall of buttons the doctrine exists to prevent.

1. **A control exists only for a step in a journey the operator takes more than
   once a day and no chord or gesture already covers.** Everything rarer has a
   home in the object's menu or on the keyboard, and costs nothing there.
2. **Secondary actions live in one context menu per object, never in a toolbar,
   and an overflow menu is not a hiding place.** Actions are found by pointing at
   the thing they act on; a "…" that collects unrelated leftovers is a junk
   drawer, not a menu.
3. **Words over icons, except the product marks.** A word needs no tooltip,
   while a mark is read instantly only because it is the harness's own.
4. **Every action has a chord, and the chord is shown where the action is.** The
   keyboard reaches everything only if the way in is visible at the point of use
   rather than in a manual.
5. **One primary action per surface.** `--accent` marks where input goes and the
   one button that acts; a second primary marks neither.
6. **A surface states its purpose in a sentence, or it is not a surface.** A tab
   that cannot say what it is for collects whatever else has no home.
7. **Nothing moves under the pointer.** A region reserves the space of its
   largest state, a list that can grow scrolls inside a fixed extent, and a
   control never shifts because a sibling appeared or vanished. A surface
   changes size only on the operator's own resize.
8. **Speed is part of the design, and a technique is chosen for the size the
   data will reach, not the size it has.** Every surface is built to be as fast
   as it can be on the weakest device it serves; a delay the operator can feel
   is a defect, and a rewrite at scale is the price of building on sand. Speed
   bought this way also lets CHROTE run on lesser hardware, which is a feature.

## The colour rule

The interface is monochrome except where colour carries meaning. Nine uses live:

| Use | Token | What the colour means |
| --- | --- | --- |
| Error and danger | `--color-error` | Something failed, or this action destroys |
| The operator's accent | `--accent` and its tints | Where you are, where input goes, what you chose, what is alive; the one button that acts (see The accent) |
| The Claude Code mark | `#D97757` | The product's own mark, kept recognisable |
| Unix-user identity | `--identity-0` … | Which account owns this session |
| Diffs | `--ansi-2`, `--color-error` | An addition, a removal; always beside a +/- gutter |
| Bead type | `--ansi-1` … `--ansi-6` | What kind of work this is; always beside the type word |
| Brain subject / collection | `--shelf-0` … `--shelf-n` | Subject identity on the map; collection identity in the rail |
| Beads closed per day | `--ansi-2` | Work finished, in the Beads strip; always beside its Created and Closed legend |
| Beads project in the work graph | `--shelf-0` … `--shelf-n` | Which store a card's Bead lives in; always named in the graph's legend |

A Bead's type takes one theme ANSI token, chosen for contrast on the theme
background, and the type word is written in it so a row reads the same without
colour: `bug` `--ansi-1`, `feature` `--ansi-2`, `chore` `--ansi-3`, `decision`
`--ansi-4`, `epic` `--ansi-5`, `task` `--ansi-6`. A type this list does not name
keeps its word in `--text-dim` rather than an invented hue. A Bead that is
closed, wont_fix or duplicate is finished arguing: its row is `--text-dim` end
to end, type, id and title alike, wherever a Bead row is drawn. An epic row is
heavier and carries a hairline beneath it, because it is the roof its children
hang from. Status stays the glyph `bd` prints, with the word on hover; in the
Beads table, where status is a column to sort and scan, the word stands beside
the glyph.

The Beads tab is one table of every Bead on the host, shown at once from each
store's last read; a store written since its read carries the read's age in the
rail, and the footer counts those stores with the oldest age while their new
reads are on their way, so stale is stated rather than hidden behind a spinner. The strip above it
counts the Beads created (`--text-dim`) and closed (`--ansi-2`) each day, with
the Beads open at each day's end as a faint area behind the bars, its edge a
faint line through each day, never a line competing with them; a tick on the
axis marks the day a store was first seen, so its arrival reads as a new store
rather than a backlog. It shows the last sixty days, wide enough to read a day;
the words beside the scale reach every day since the first store began. A drag
across the strip brushes the days the table keeps. The rail keeps two filters,
project and status, each row a box that toggles it and a name that keeps it
alone; a store is named for its prefix, or for its role when that says more,
as the brain does. Ids are never cut: the column is as wide as the longest
shown, up to fifteen characters, and a longer id wraps after a dot. Beside it
the type is its word in its colour, the sixth colour rule, so a bug and an epic
read apart down the column. Numeric
columns are right-aligned with a faint bar beside the number; when the table
is too narrow for every column, Updated gives way to Idle, which sorts the
same, and Size goes. The work graph draws the selected Bead's family: its cards' borders carry
their project's shelf colour, finished cards are dimmed and blocked cards
dashed. A long chain wraps into bands one under the next, or runs down a tall
pane, and a small family shrinks a little below the readable scale rather
than leave a card out of view. The view is List or Map. In List, nothing
selected, the table has the pane; a selection puts the graph and the Bead's own
words beside the table, or beneath it when the two no longer fit side by side,
and `Escape` in the table lets it go. The text sits beside the graph in a wide
pane and beneath it in a tall one. Map is the slice as the Brain's sky: each
epic, or any Bead with children, a system with the Bead at its centre, its open
work in orbit and its finished work the nebula, so how much is done shows as
glow; Beads under no parent drift around their project's Unfiled system, and
parent and blocker links are the lines. A star selects its Bead, whose words
come in beside the sky. The map places every Bead of the projects kept, so a
status or a search fades stars rather than moving them. The target it was built to is in
[`docs/assets/beads-slicer/`](docs/assets/beads-slicer/final-target.png).

The Services tab is one row per service in the host's catalog order, so
nothing moves while states change. A light leads each row: the accent's dot
when it runs (what is alive), half filled when part of it runs, a hollow ring
when stopped, dashed when the controller cannot say, `--color-error` when it
failed, and breathing while it starts or stops. The state follows in a word,
then Open when the service has a page, then the row's one switch, a word
button that says what it will do: Start, or Stop when it runs. Restart, Open
and Send are in the row's menu. A pressed switch gives way to the state's own
words, Starting… or Stopping…, until the controller has answered and a fresh
read agrees, and a failure stays on its row in the controller's words. A
read-only service says Read-only where the switch would be. The tab opens on
the list it already has: the pointer reaching the tab asks for it.

Everything else is gone: per-window accent colours, hashed per-user hues, the
tmux status bar hue, theme-button glows, and per-severity notification colours.
A status line reporting a failure uses the error token; every other line is
plain.

Identity colours come from the theme, assigned to the server's `terminalUsers`
in order so a user is the same colour on every device. A user the server does
not list gets `--text-dim` rather than an invented colour.

Brain colours come from the theme's shelf palette. A subject's path selects its
colour deterministically, so adding another subject does not recolour existing
ones. The collection rail retains collection colours. Colours may repeat;
labels identify the group. Circles mark topics and notes, squares research,
and diamonds sources. The accent marks the selected or pointed-at page.

## The accent

The accent is a voice, not a decoration. It appears in five roles and nowhere
else, the same way on every surface, so that once learned it reads at a
glance:

| Role | How it is drawn |
| --- | --- |
| Where you are | The current tab, choice or step underlined in `--accent`; the current row in a list has a 2px `--accent` bar at its left and the `--accent-soft` wash |
| Where input goes | A focused field's hairline, the focus ring, the terminal cursor and selection |
| What you chose | A toggle's filled mark, a picked option's border and wash, a word that is on (`aria-pressed`) in `--accent` |
| What is alive | A small `--accent` dot: a project an agent works in, a resident or tab with news; the live line of a chart |
| The one action | One primary button per surface, filled with `--accent`, its words in `--on-accent` |

Hover on a word is the `--accent-soft` wash, never a border. The accent never
colours a heading, a title, a decorative border, a large area or a second
button on the same surface; a surface with two primaries has none. Danger is
`--color-error`, never the accent.

## Typeface

One family for chrome and terminal alike, because the face is part of
CHROTE's character: JetBrains Mono, with CHROTE Term
Symbols behind it — a subset of Iosevka Term, renamed because Iosevka is a
reserved font name — covering the box drawing, braille and frame glyphs agent
TUIs paint with. Both are `woff2` files served from CHROTE's own origin, so no request for a
font leaves the host. Prose (a brain page, a Bead's description, a Markdown
file) is set a notch larger than the chrome, at a measure of about 70
characters. There is no font picker; font size stays a
device-local setting.

## Control kit

Every control comes from `dashboard/src/components/kit/`, never from the
browser: the browser's checkbox, select and 13.33px button face do not appear.

- **Toggle** is on or off: a hollow square that fills with the accent. With
  words it is a whole row that toggles, count and all; bare, it is the mark in a
  28px square, for a row whose name already does something else.
- **Choice** picks one of a few: the current choice in words over a hairline,
  opening the one menu with the choices as rows and the current one checked.
- **Button** (`kit-button`) is a word that acts: quiet text that brightens on a
  soft accent wash. `primary` fills it with the accent for the one action that
  matters on a surface; `danger` speaks in the error colour, and a two-press
  confirmation shows `armed` in its wash. A chord beside a word is
  `kit-chord`. There are no outlined buttons and no icon buttons: a cross is the
  word Close, an arrow is a step in a path.
- **Tabs** (`kit-tabs`) choose between a few views or states in words, the
  current one underlined in the accent: the Sessions and Files switch, List and
  Map, the strip's span, a schedule's kind.
- **Row** (`kit-row`) is a line in a list that acts as a whole, with the
  current one marked as everywhere: the accent's bar and its wash.
- **Label** (`kit-label`) is the quiet capitals over a section.
- **PathTrail** is where the operator stands as folders to step back along,
  starting at the project when inside one; it replaces Up and the path line.
- **SearchField** is the one search box: 28px over a hairline that takes the
  accent while typing, its placeholder saying what it searches as "Search …",
  with a quiet hint at its right for a chord or a count. What a key does beyond
  typing, such as Down into the results, belongs to the view that owns them.
- **Range** (`kit-range`) is a hairline with a small square thumb.

Every one is at least 28px in each direction, and 44px under a coarse
pointer. A control that must take less room in a dense row keeps its 28px
target and reaches into the gap beside it, as the file tree's expand toggle
does.

## Theme

One active theme at a time, and no picker. The host authors it, the server
serves it, the dashboard applies it. Changing the look is an apply on the host
plus a browser reload; the browser holds no theme state.

- **Source.** A theme directory in operator configuration. A host apply script
  validates it and copies it to the directory named by `CHROTE_THEME_DIR`.
- **Route.** `GET /api/theme` serves that document verbatim, the embedded
  default when the host authored none, and a 500 naming the offending field when
  a theme exists but does not validate. `GET /api/theme/art/{name}` serves art.
- **Consumers.** The dashboard's CSS custom properties, the terminal palette,
  the empty-window art, and the identity colours. The tmux status bar and each
  agent's own theme are written once by the host apply script in ANSI colour
  names, so an SSH client renders them in its own palette. CHROTE never writes
  host-global tmux state to style anything.

The served document is schema 1: `ui`, `terminal` with exactly sixteen ANSI
colours, `identity`, `shelves`, and `art`. `shelves` is optional, because a
theme authored before the map had colour has none and the built-in palette
answers for it. Every colour is `#rrggbb` or `#rrggbbaa`.
[`src/internal/api/theme_default.json`](src/internal/api/theme_default.json) is
the reference document and the served fallback;
[`docs/installation.md`](docs/installation.md) documents the environment.

The reference theme is `chrote-violet`: near-neutral grounds with a faint
violet cast, so the accent belongs to them, and the accent `#a68cff` at 7:1 on
the background. The terminal's magenta is moved toward pink so a Bead epic
(`--ansi-5`) is never mistaken for the accent, and no identity colour is
violet. A host theme sets its own accent; the roles above stay the same.

### Colour token model

The theme writes these custom properties on `:root`:

```css
--background  --surface-primary  --surface-secondary  --divider
--text-primary  --text-secondary  --text-dim
--accent  --accent-light  --accent-soft  --on-accent  --accent-rgb
--color-error  --color-error-light
--terminal-background  --terminal-foreground  --ansi-0 … --ansi-15
--identity-0 … --identity-n  --shelf-0 … --shelf-n
```

Components use tokens rather than hard-coded colours. There are no per-window
colour tokens and no glow tokens.

## Tiles

- **Focus.** A focused tile changes its border colour to `--accent` and nothing
  else. The border does not change width, so nothing moves when focus lands. No
  hover effect, no glow, no transform.
- **Header versus status line.** The header carries identity: session name,
  harness mark, Unix user, tile state. What the agent is doing is the agent's
  own to report, through its status line and its pane title. CHROTE knows only
  the pane command and never dresses that up as an internal state.
- **Tag label.** Truncation preserves the tail: the head shortens with an
  ellipsis, the last segment after the final hyphen stays whole, and the full
  name is in the title attribute. A window with one bound session keeps the
  full-width label.
- **Harness mark.** `claude` shows the Claude Code mark and `codex` the Codex
  mark. A shell shows nothing, because a shell is the resting state. Any other
  command shows its own name as dim text, no prefix and no border.
- **Stability.** Panes stay put through session changes, and a control the
  operator hits repeatedly does not move under him.

## Launcher

An empty window is the launcher, drawn over the theme's art for that slot. It
offers the four choices a launch needs: harness, folder, Unix user, and name.
The Sessions panel's plus opens the same launcher. Short wide tiles arrange
the controls in two columns; narrow tiles keep one column. Overflow scrolls
from the first control, so both harness selection and Launch remain reachable.

- Harnesses, their default flags and their flag catalogues are operator
  configuration read through `GET /api/launch`. The binary stays on the server.
  Flags are an editable line prefilled with the harness's defaults, a preview
  shows the exact line that will be typed, and a panel lists every flag the
  harness's own `--help` describes, searchable, click to add or remove.
- The folder is one field over a list of six rows that keeps its height
  whatever it holds. Empty, the list is the pinned folders and the working
  directories of live sessions; a fragment ranks the host's workspaces by fuzzy
  match on the tail of their paths; an absolute path lists the directories
  under the typed prefix, dot-directories last. Tab completes the highlighted
  row, the arrows move, Enter takes the highlighted row or the typed path.
- The name is derived from folder and harness, suffixed on collision, and
  editable before launching.
- One quiet button states what will happen, and the new session binds to the
  window it was launched from.

## Keyboard

A focused terminal forwards every key to the program inside it, so CHROTE takes
one modifier and nothing else. **`Alt` is the CHROTE key.** A registered `Alt`
chord runs its action wherever the cursor is; every other `Alt` combination
reaches the pty untouched, and so does `AltGr`, which a Finnish layout needs for
`@`, `$`, `{` and `}`.

| Chord | Action |
| --- | --- |
| `Alt+1` … `Alt+6` | Terminal tab n, for the tabs that exist |
| `Alt+B` | Beads column |
| `Alt+L` | Brain tab |
| `Alt+G` | Agents tab |
| `Alt+W`, `Alt+Shift+W` | Next and previous window in the active tab |
| `Alt+Plus`, `Alt+Minus` | Add a window; remove the last empty one |
| `Alt+N` | Launcher in the focused window |
| `Alt+S` | Send to Session for the focused tile; in a tab with a resident, paste the table's reference into its prompt |
| `Alt+A` | Sessions panel |
| `Alt+O` | Files panel |
| `Alt+I` | Close the table |
| `Alt+Enter` | Focus the resident's drawer |
| `Alt+K` | Keybindings panel |
| `Alt+R` | Report an issue: file a bug or feature as a Bead, and optionally hand it to an agent |

Leader then `B` opens the Beads tab. It has no direct chord because `Alt+B`
belongs to the Beads column on every tab.

`Plus` and `Minus` are matched by the character the layout produces, not by
`Shift`, so `Alt++` and `Alt+=` are the same chord.

- **Leader.** `Ctrl+Shift+Space` is discovery, not the daily path. It is taken
  before the key reaches the pty and again at document level, so it answers
  inside a focused tile as well as outside one, and it opens the panel below.
- **Toggle.** A text button at the right of the tab bar reads `Keys on` or
  `Keys off`: device-local, on by default, and off means nothing is intercepted.
- **Panel.** The leader and `Alt+K` open the same centred list of every chord in
  scope, as `CHORD → action`. Typing filters it, `Enter` runs the current row.
- **Toggles.** A chord that opens a glance closes it when pressed again: the
  leader and `Alt+K` on the keybindings panel.
- **Echo.** A registered chord that fires shows its key caps at the foot of the
  workspace for 800 ms, in the badge a confirmation also wears: the modifier is
  the filled cap and the key the outlined one. Nothing else echoes, because
  nothing else was taken.
- **Scopes.** `global` chords list always, `workspace` chords while a terminal
  tab is active, `tile` chords while a tile is focused. Scope decides what the
  panel shows and what a chord can reach.
- Plain `/` and `?` keep working outside a terminal, as they do today.

## Hand-off

Send to Session is the one way to hand work to an agent: the drawer for any
session, and in a tab with a resident a paste straight into the resident's
prompt. Every object that can be handed over opens it: a tile, a session row,
Peek, a file, a diff, a Bead, a library page, an annotated element.

- **A drawer, not a modal.** It draws from the right edge at 380px over the
  table's column, with no gutter, so nothing beneath it moves and what was
  beneath is there again when it closes.
- **A reference, then the note.** Each entry point passes one line the agent can
  act on — a path, a Bead id and title, a library page, a component and its file.
  The drawer shows that line first, styled read-only, and puts the cursor in the
  note beneath it.
- **A target.** The focused tile's session by default, a searchable picker
  otherwise, and a "new agent" entry that opens the launcher with the message.
- **One primary action.** Send pastes and submits. Pasting without submitting is
  the secondary action, for a prompt the operator wants to read before it runs.
- **A resident takes the paste.** Where the tab has a resident agent, `Alt+S`
  and the object's Send put the table's reference into the resident's prompt
  without submitting, and the operator finishes the sentence there. The drawer
  serves every other session.
- **Phone composition.** Below 769px the drawer fills the visible viewport,
  including when the keyboard reduces its height. A target summary with Change
  replaces the always-visible picker; changing targets preserves the draft.
  Return adds a line. Send and Paste remain explicit buttons, reachable below
  the scrolling content on every screen size. Desktop keeps its remembered
  drawer width and keyboard shortcuts.
- **A receipt.** The tile scrolls to the bottom and the drawer closes; a failure
  keeps the drawer open with the server's own message.

**Dev mode** is the hand-off for the dashboard's own faults. It is a chord away
in the keys panel; while it is on, a highlight follows the pointer, a corner
label names the component under it and the file it is written in, and a click
opens the drawer with that line already written instead of pressing whatever it
landed on. The identity in the line comes from `data-ui="<surface>.<part>"`,
which the surfaces worth naming carry by hand, and from the component name,
which the build is told to keep through minification. The target picker offers a
new CHROTE agent alongside the live sessions, because a complaint about CHROTE
usually has no session waiting for it.

## The right edge

Three things share the right edge of a tab, in layers: the table, the Send
drawer over it, and the resident's drawer beyond both.

- **The table** holds the one selected object: a Bead, a page, what an agent
  sees. It is global, so the object chosen in one tab is on the table in every
  tab, and each tab shows it where it fits: the Beads tab as the detail column
  beside its table, a terminal tab as a column at the right of the grid that
  stays through tab switches. The column is drag-resizable at its left edge,
  400px by default and never narrower than 320px, and the width is remembered
  on the device. It narrows the content beside it and never overlays it.
  `Alt+I`, `Escape` and its Close word clear it.
- **The Send drawer** overlays the table's column from the right edge at 380px
  with no gutter, so nothing beneath it moves. Its own border and surface say
  that a layer is beneath; no strip of cut words does. It closes after Send and
  on `Escape`, and what was beneath is there again.
- **The resident's drawer** is a tmux session that lives in its tab: the
  Librarian in Brain, the tender in Agents, the Clerk in Beads. Closed, it is
  a 28px spine at the far right of the tab carrying the harness mark, the
  label and the state, with an accent dot when the resident has news. A click,
  `Alt+S` or `Alt+Enter` slides it in over the content, never over the table, so
  the object being handed over stays in sight, 44 columns wide by
  default, drag-resizable and remembered, under a 32px header carrying the
  label, the harness mark, the session name, its state as a word, Send with
  its chord, and Close, which puts it back. The spine keeps its place while
  the drawer is open, so nothing beneath it moves. The terminal is
  the input; there is no ask line. `Alt+S` in the tab pastes the table's
  reference into the resident's prompt (in Beads, the selected row's Bead
  when there is one) without submitting it, and the operator
  types the rest and presses `Enter` there; the drawer serves every other
  session. When the session is absent, the drawer shows Launch with the
  resident's folder. Open or closed is remembered per tab, and the session
  persists, so leaving and returning finds the agent mid-answer. The
  header's menu offers Peek, Send, Launch and Restart. Which session and folder
  each resident uses is operator configuration, and each resident's charter
  lives on the host outside this repository.

When space runs out, the content keeps 480px first and the table shrinks to its
minimum; the resident's spine always keeps its 28px. A rail narrows to its longest name, between 120 and 200px.

## Floating surfaces

A surface that opens over or beside the workspace is one of two kinds, told
apart by how it closes.

- **A glance goes away when you look elsewhere.** Peek, the keybindings panel,
  every menu, the launcher while nothing has been typed into it, and the image
  glance. A click outside closes it, and that click is consumed rather than
  passed through; `Escape` closes it from anywhere, a focused terminal
  included, except Peek's own terminal; the chord that opened it closes it.
- **A work surface stays until you close it.** The Send drawer, an editor, the
  Bead card and What this agent sees. `Escape`, its Close word or its chord
  closes it, and a click outside means what that click meant. A drawer holding
  typed text confirms in place before it discards.

`Escape` belongs to the topmost open surface. It reaches the pty only when
nothing is open, or when it is typed into Peek's own terminal, where the
operator is driving the session.

A menu is a flat sheet of words attached flush to the edge of the control that
opened it: the action with its chord at the right, hairlines between groups, no
icons, no radius and no blur, and the highlighted row taking
`--surface-secondary` and a 2px `--accent` bar.

Peek is a floating window centred over the workspace and sized by the session
it shows: the tmux window's own columns and rows, status line included, at the
tile font, capped at 90% of the workspace in each direction. It always shows
the whole window: past the cap, or when the operator resizes it from an edge
or corner, the font shrinks and the grid stays. It opens with its terminal
focused, so the session can be driven from it. A one-line header carries the
mark, the name, Send and Close as words. It is not dragged.

The image glance is Peek's manner for a picture: the path, the pixel size, and
Open in Files, Copy path and Close as words. The image fits 90% of the
workspace and is never upscaled. It opens from an image path link in a terminal
and from a click on an image in the Files panel or tab.

An announcement that confirms the operator's own action, or reports a failure,
is a toast in the bottom-centre slot the key echo uses; information such as a
load or a count takes the status line only. The confirmation is the echo's
badge with words: a cap for the kind, then the message at cap weight. A
confirmation fills the cap with `--accent`, a warning leaves it outlined, and a
failure fills it with `--color-error` and colours the words the same. A copy
that lands is confirmed this way wherever it was made, the painted terminal
selection included. The toast
fades in over 120 ms, holds for 1800 ms and fades out over 200 ms, one at a
time with the newest replacing the last. The status line, a 28px footer across the full width of the window
beneath both the Sessions panel and the workspace, keeps the last event with
its time as the record, so nothing is lost when the toast has gone; only a
failure takes colour there. When an agent finishes or needs input, its
harness's own hook tells CHROTE, and the telling is a mark on the session's row
and its tab until the session is focused, the toast naming the session, and,
opt in on the device, a short tone for each kind of event and a browser
notification while the tab is hidden.

Nothing asks a question in a dialog: a destructive control reads its own
confirmation until a second press within three seconds runs it, a rename
happens in the object's own place, and neither `window.prompt` nor
`window.confirm` appears anywhere. No scanlines and no ornament: light and motion
appear only where they carry meaning or explain a change.

## Copy tone

CHROTE can be blunt and characterful, but operational surfaces stay short.
Personality lives in empty windows and documentation. Errors, secrets, unsafe
actions, and recovery stay literal, and nothing CHROTE writes is mixed into live
terminal text.
