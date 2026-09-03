---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Design System Spec

Status: **Active core source of truth**.

CHROTE is a dense private cockpit: an instrument panel for real work, legible
and honest about state. Nothing decorative competes with the terminal.

## Design principles

1. **Cockpit density.** Show useful state; do not bury it in cards or copy.
2. **Host-state first.** Make clear what is durable host state and what is
   browser preference.
3. **Local directness.** Context menus, drag/drop, inline popovers, and visible
   state beat modal-heavy workflows.
4. **Failure visibility.** Empty, degraded, blocked, and errored states are
   explicit and actionable.
5. **Colour is information.** It marks meaning, never mood.
6. **Recognition over labels.** A known agent shows its mark; anything else is
   shown in its own words, untranslated.
7. **Accessible contrast.** Themes may differ; state and text stay readable.

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

## The colour rule

The interface is monochrome except where colour carries meaning. Five uses live:

| Use | Token | What the colour means |
| --- | --- | --- |
| Error and danger | `--color-error` | Something failed, or this action destroys |
| Focus and the primary action | `--accent` | Where input goes; the one button that acts |
| The Claude Code mark | `#D97757` | The product's own mark, kept recognisable |
| Unix-user identity | `--identity-0` … | Which account owns this session |
| Diffs | `--ansi-2`, `--color-error` | An addition, a removal; always beside a +/- gutter |

Everything else is gone: per-window accent colours, hashed per-user hues, the
tmux status bar hue, theme-button glows, and per-severity notification colours.
A status line reporting a failure uses the error token; every other line is
plain.

Identity colours come from the theme, assigned to the server's `terminalUsers`
in order so a user is the same colour on every device. A user the server does
not list gets `--text-dim` rather than an invented colour.

## Typeface

One family for chrome and terminal alike: JetBrains Mono, with CHROTE Term
Symbols behind it — a subset of Iosevka Term, renamed because Iosevka is a
reserved font name — covering the box drawing, braille and frame glyphs agent
TUIs paint with. Both are `woff2` files served from CHROTE's own origin, so no
request for a font leaves the host. There is no font picker; font size stays a
device-local setting.

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
colours, `identity`, and `art`. Every colour is `#rrggbb` or `#rrggbbaa`.
[`src/internal/api/theme_default.json`](src/internal/api/theme_default.json) is
the reference document and the served fallback;
[`docs/installation.md`](docs/installation.md) documents the environment.

### Colour token model

The theme writes these custom properties on `:root`:

```css
--background  --surface-primary  --surface-secondary  --divider
--text-primary  --text-secondary  --text-dim
--accent  --accent-light  --accent-rgb
--color-error  --color-error-light
--terminal-background  --terminal-foreground  --ansi-0 … --ansi-15
--identity-0 … --identity-n
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
The Sessions panel's plus opens the same launcher.

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
| `Alt+B` | Beads tab |
| `Alt+L` | Library tab |
| `Alt+W`, `Alt+Shift+W` | Next and previous window in the active tab |
| `Alt+Plus`, `Alt+Minus` | Add a window; remove the last empty one |
| `Alt+N` | Launcher in the focused window |
| `Alt+S` | Send to Session for the focused tile |
| `Alt+P` | Peek the focused tile's session |
| `Alt+A` | Sessions panel |
| `Alt+O` | Files panel |
| `Alt+K` | Keybindings panel |

`Plus` and `Minus` are matched by the character the layout produces, not by
`Shift`, so `Alt++` and `Alt+=` are the same chord.

- **Leader.** `Ctrl+Shift+Space` is discovery, not the daily path. It is taken
  before the key reaches the pty and again at document level, so it answers
  inside a focused tile as well as outside one, and it opens the panel below.
- **Toggle.** A text button at the right of the tab bar reads `Keys on` or
  `Keys off`: device-local, on by default, and off means nothing is intercepted.
- **Panel.** The leader and `Alt+K` open the same centred list of every chord in
  scope, as `CHORD → action`. Typing filters it, `Enter` runs the current row.
- **Echo.** A registered chord that fires shows its key caps at the foot of the
  workspace for 800 ms. Nothing else echoes, because nothing else was taken.
- **Scopes.** `global` chords list always, `workspace` chords while a terminal
  tab is active, `tile` chords while a tile is focused. Scope decides what the
  panel shows and what a chord can reach.
- Plain `/` and `?` keep working outside a terminal, as they do today.

## Hand-off

Send to Session is the one way to hand work to an agent. Every object that can
be handed over opens it: a tile, a session row, Peek, a file, a diff, a Bead, a
library page, an annotated element.

- **A drawer, not a modal.** It docks at the right and the grid narrows to fit,
  so the tile it targets stays visible; only a narrow viewport makes it overlay.
- **A reference, then the note.** Each entry point passes one line the agent can
  act on — a path, a Bead id and title, a library page, a component and its file.
  The drawer shows that line first, styled read-only, and puts the cursor in the
  note beneath it.
- **A target.** The focused tile's session by default, a searchable picker
  otherwise, and a "new agent" entry that opens the launcher with the message.
- **One primary action.** Send pastes and submits. Pasting without submitting is
  the secondary action, for a prompt the operator wants to read before it runs.
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

## Floating surfaces

A menu is a flat sheet of words attached flush to the edge of the control that
opened it: the action with its chord at the right, hairlines between groups, no
icons, no radius and no blur, and the highlighted row taking
`--surface-secondary` and a 2px `--accent` bar. A sheet docks to an edge and
takes a share of the workspace with no backdrop — Peek at the left, snapped to
the nearest tile boundary at or below 60% so no tile is cut mid-glyph — so
`Escape` closes it while a click outside still means what that click meant.
Every announcement lands on the status line, a 28px footer across the full width
of the window beneath both the Sessions panel and the workspace, carrying the
last event with its time, where only a failure takes colour. Nothing asks a
question in a dialog: a destructive control reads its own confirmation until a
second press within three seconds runs it, a rename happens in the object's own
place, and neither `window.prompt` nor `window.confirm` appears anywhere. No
scanlines, no glow, no decorative motion: motion survives only where it confirms
an action.

## Copy tone

CHROTE can be blunt and characterful, but operational surfaces stay short.
Personality lives in empty windows and documentation. Errors, secrets, unsafe
actions, and recovery stay literal, and nothing CHROTE writes is mixed into live
terminal text.
