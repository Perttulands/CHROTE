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

## The colour rule

The interface is monochrome except where colour carries meaning. Four uses live:

| Use | Token | What the colour means |
| --- | --- | --- |
| Error and danger | `--color-error` | Something failed, or this action destroys |
| Focus and the primary action | `--accent` | Where input goes; the one button that acts |
| The Claude Code mark | `#D97757` | The product's own mark, kept recognisable |
| Unix-user identity | `--identity-0` … | Which account owns this session |

Everything else is gone: per-window accent colours, hashed per-user hues, the
tmux status bar hue, theme-button glows, and per-severity toast colours. A toast
reporting a failure uses the error token; every other toast is plain.

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

## Launcher

An empty window is the launcher, drawn over the theme's art for that slot. It
offers the four choices a launch needs: harness, folder, Unix user, and name.
The Sessions panel's plus opens the same launcher.

- Harnesses, their default flags and their flag catalogues are operator
  configuration read through `GET /api/launch`. The binary stays on the server.
  Flags are an editable line prefilled with the harness's defaults, a preview
  shows the exact line that will be typed, and a panel lists every flag the
  harness's own `--help` describes, searchable, click to add or remove.
- Folders are the configured pinned list, the working directories of live
  sessions as recents, and a browse control.
- The name is derived from folder and harness, suffixed on collision, and
  editable before launching.
- One quiet button states what will happen, and the new session binds to the
  window it was launched from.

## Effects

No scanlines, no glow, no decorative motion. Modals keep a plain shadow. Motion
survives only where it confirms an action.

## Layout rules

- Keep terminal panes stable during session changes.
- Avoid layout shifts in controls that users hit repeatedly.
- Preserve visible context around destructive actions.
- Use sidebars/panels for persistent operational surfaces.
- Use popovers/context menus for local edits.
- Prefer progressive detail over wall-of-text help in the main workspace.

## Copy tone

CHROTE can be blunt and characterful, but operational surfaces stay short.
Personality lives in empty windows and documentation. Errors, secrets, unsafe
actions, and recovery stay literal, and nothing CHROTE writes is mixed into live
terminal text.
