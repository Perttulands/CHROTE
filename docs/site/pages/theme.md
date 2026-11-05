---
title: Theme
nav: Theme
summary: One active theme, authored on the host and served by CHROTE. The browser stores none and offers no picker.
sources: DESIGN-SYSTEM.md, docs/installation.md, src/internal/api/theme_default.json
---

# Theme

CHROTE has one active theme at a time. The host authors it, the server serves
it, and the dashboard applies it to its chrome, the terminal palette, the
empty-window art and the per-user identity colours. Changing the look is an
apply on the host and a browser reload.

## The default

Unset, CHROTE serves its embedded reference theme, **chrote-violet**:
near-neutral grounds with a faint violet cast and the accent `#a68cff`. The
document is [`theme_default.json`](../../../src/internal/api/theme_default.json),
and it is the best starting point for your own. This documentation site uses the
same palette.

## Your own theme

Set `CHROTE_THEME_DIR` to a directory holding `theme.json` and, beside it, an
`art/` directory:

```text
my-theme/
├── theme.json
└── art/
    └── dunes.webp
```

```json
{
  "schema": 1,
  "name": "example-dark",
  "ui": {
    "background": "#0f0f0f", "surface": "#1a1a1a", "surfaceRaised": "#252525",
    "divider": "#3a3a3a", "text": "#e5e5e5", "textSecondary": "#a3a3a3",
    "textDim": "#737373", "accent": "#6b9fff", "error": "#f87171"
  },
  "terminal": {
    "background": "#0a0a0a", "foreground": "#e5e5e5", "cursor": "#e5e5e5",
    "selectionBackground": "#6b9fff40",
    "ansi": [
      "#0f0f0f", "#f87171", "#8bd450", "#e5c07b", "#6b9fff", "#c084fc", "#45d6d6", "#a3a3a3",
      "#737373", "#ff8a8a", "#a6e37a", "#f0d48a", "#8fb5ff", "#d3a4ff", "#7ae2e2", "#ffffff"
    ]
  },
  "identity": ["#4f6d8f", "#8f6f3a"],
  "art": ["dunes.webp"]
}
```

| Rule | Detail |
| --- | --- |
| Colours | `#rrggbb` or `#rrggbbaa` |
| `terminal.ansi` | Exactly sixteen colours |
| `identity` | At least one; assigned to the configured terminal users in order, so a user is the same colour on every device |
| `shelves` | Optional colours for Brain subjects and Beads projects; omitted, the built-in palette answers |
| `art` | Names matching `[A-Za-z0-9._-]+`, no path separators, served from `art/` |

## What the colours mean

CHROTE is monochrome except where colour carries meaning, and your theme keeps
those meanings. The **accent** marks *you*: where you are, where input goes,
what you chose, what is alive, and the one button that acts. **Error** is
failure and danger. The ANSI colours type Beads (bug, feature, chore, decision,
epic, task use ANSI 1 to 6), and identity colours mark which Unix user owns a
session. Keep text and state readable on your grounds. The full rule is in
[`DESIGN-SYSTEM.md`](../../../DESIGN-SYSTEM.md#the-colour-rule).

## Check it

```bash
curl -i http://127.0.0.1:8094/api/theme
```

- **200 with your `name`**: served. Reload the dashboard; it reads the theme at
  load.
- **200 with `chrote-violet` and no art**: the embedded default.
  `CHROTE_THEME_DIR` is unset, or holds no `theme.json`.
- **500 naming a field**: `theme.json` exists but does not validate. Fix the
  field. CHROTE does not fall back here, so a broken edit cannot pass for a
  deliberate palette.

## tmux and the agents' own colours

CHROTE never writes host-wide tmux state to style anything. If you want the tmux
status bar and each agent's own theme to match, set them once from your theme
tooling, in ANSI colour names, so an SSH client renders them in its own palette.
