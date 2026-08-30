---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Design System Spec

Status: **Active core source of truth**.

CHROTE is a dense private cockpit, not a marketing page and not a sterile SaaS
admin panel. It should feel like an instrument panel for real work: legible,
fast, slightly theatrical, and hard to mistake for a toy.

## Design principles

1. **Cockpit density.** Show useful state without hiding it behind decorative
   cards or long instructional copy.
2. **Host-state first.** UI elements should make clear what is durable host state
   versus browser preference.
3. **Local directness.** Context menus, drag/drop, inline popovers, and visible
   state beat modal-heavy workflows.
4. **Failure visibility.** Empty, degraded, blocked, and errored states should be
   explicit and actionable.
5. **No fake calm.** Agentic work is messy. The UI can have character without
   obscuring operational truth.
6. **Accessible contrast.** Themes may differ, but state and text must remain
   readable.

## Theme ids

Current dashboard themes:

| Theme id | Use |
| --- | --- |
| `matrix` | High-contrast green-on-dark terminal energy |
| `dark` | Neutral default cockpit |
| `gastown` | Warm amber/russet cockpit palette |

Theme ids are persisted settings. Rename them deliberately and test migration or
fallback behavior.

## Color token model

Themes provide CSS custom properties:

```css
--background
--surface-primary
--surface-secondary
--divider
--text-primary
--text-secondary
--text-dim
--accent
--accent-light
--accent-glow
--color-error
--color-error-light
--color-error-glow
--accent-rgb
--window-blue
--window-purple
--window-green
--window-orange
```

Components should use tokens rather than hard-coded theme-specific colors.

## Layout rules

- Keep terminal panes stable during session changes.
- Avoid layout shifts in controls that users hit repeatedly.
- Preserve visible context around destructive actions.
- Use sidebars/panels for persistent operational surfaces.
- Use popovers/context menus for local edits.
- Prefer progressive detail over wall-of-text help in the main workspace.

## Copy tone

CHROTE can be blunt and characterful, but operational surfaces should stay short.
Use humor around empty states and docs; use clarity around errors, secrets,
unsafe actions, and recovery.
