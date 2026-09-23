---
name: chrote-theme
description: Author, validate, preview, and apply a CHROTE colour theme on this host. Use when asked to change CHROTE's colours, add a theme, or adjust the terminal palette.
---

# Author a CHROTE theme

CHROTE serves one theme, read by the Go server from `$CHROTE_THEME_DIR` and
handed to the browser over `GET /api/theme`. Themes are authored on this host
under `/srv/ops/chrote-host/themes/<name>/`, never inside `/srv/chrote`.
Read `/srv/ops/chrote-host/RUNBOOK.md` before applying anything.

## The schema

`themes/chrote-dark/theme.json` is the worked example; copy it and edit.
Schema 1 requires:

- `schema` is `1` and `name` is a non-empty string.
- `ui` holds `background`, `surface`, `surfaceRaised`, `divider`, `text`,
  `textSecondary`, `textDim`, `accent`, `error`.
- `terminal` holds `background`, `foreground`, `cursor`,
  `selectionBackground`, and `ansi` with exactly 16 entries.
- `identity` holds at least one colour, assigned to Unix users by their
  position in the server's `terminalUsers` order.
- Every colour is `#rrggbb` or `#rrggbbaa`.
- `art` names files in the theme's `art/` directory and each name matches
  `^[A-Za-z0-9._-]+$`. WebP, longest side 1600px, under 400 KB each.

## Picking a palette

CHROTE's design system allows no decorative colour: tiles are monochrome,
liveness is text, and colour carries meaning or is absent.

- Build the interface from `background`, `surface`, `surfaceRaised` and
  `divider` as near-neutral steps, and let the three text tones do the work.
- Spend `accent` only on focus and selection, and `error` only on errors.
- The 16 `ansi` entries are the terminal's own palette. Keep them
  recognisable as the standard eight and their bright pair.
- `identity` colours should be muted and clearly distinct from `accent`.

Contrast is enforced, not advisory. `text` and `textSecondary` need 4.5:1 on
both `background` and `surface`, `textDim` needs 3:1 on both, and
`terminal.foreground` needs 4.5:1 on `terminal.background`.

## Check it

Set `theme_name` to the folder name under `themes/`.

```bash
cd /srv/ops/chrote-host
./bin/chrote-theme-check "$theme_name"
```

It prints every contrast pair with its ratio and threshold, and exits non-zero
on a schema violation or a pair below its minimum. Fix the theme until it
passes; do not lower a threshold.

## Preview it

Point a development dashboard at the theme without applying it. Stage it into
a scratch directory, set `CHROTE_THEME_DIR` to that directory for a dev server
you start yourself, and load the dashboard. Never preview against the deployed
`chrote-srv`, and never restart it.

## Apply it

Set `target_socket` to the intended tmux socket and `target_user` to its user.

```bash
./bin/chrote-theme-apply "$theme_name" --dry-run --tmux-socket "$target_socket" --user "$target_user"
```

Read that output in full. It lists every file apply would write and every
command it would run, and touches nothing. Only when it is right, run the same
command without `--dry-run`.

Applying is an operator step. This host runs live tmux sessions holding real
work. Apply writes into real home directories and sets options individually
on each selected live server. New servers load the generated theme through
the user's tmux configuration. Do not run apply against a live socket or
another user's home on your own initiative. Inspect the dry run first.

Rollback, the environment lines the unit needs, and the per-user permission
limits on this host are all in the RUNBOOK.
