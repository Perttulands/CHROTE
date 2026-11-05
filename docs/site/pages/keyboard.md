---
title: Keyboard
nav: Keyboard
summary: Alt is the CHROTE key. Every other key, AltGr included, reaches the program in the terminal.
sources: DESIGN-SYSTEM.md, docs/journeys.md, dashboard/src/keys/useAppChords.ts
---

# Keyboard

A focused terminal forwards every key to the program inside it, so CHROTE takes
one modifier and nothing else. **`Alt` is the CHROTE key.** A registered `Alt`
chord runs its action wherever the cursor is, a focused terminal included.
Every other `Alt` combination reaches the program untouched, and so does
`AltGr`, which some layouts need for `@`, `$`, `{` and `}`.

## Chords

| Chord | Action | Where |
| --- | --- | --- |
| `Alt+1` … `Alt+6` | Terminal tab n, for the tabs that exist | Anywhere |
| `Alt+B` | Toggle the Beads column | Anywhere |
| `Alt+L` | Brain tab | Anywhere |
| `Alt+G` | Agents tab | Anywhere |
| `Alt+I` | Close the table | Anywhere |
| `Alt+Enter` | Focus the tab's resident | Anywhere |
| `Alt+C` | Claim all sessions on this device | Anywhere |
| `Alt+K` | Keybindings panel | Anywhere |
| `Alt+R` | Report an issue as a Bead | Anywhere |
| `Alt+Shift+D` | Dev mode | Anywhere |
| `Alt+W`, `Alt+Shift+W` | Next and previous window | Terminal tab |
| `Alt+Plus`, `Alt+Minus` | Add a window; remove the last empty one | Terminal tab |
| `Alt+N` | Launcher in the focused window | Terminal tab |
| `Alt+A` | Sessions panel | Terminal tab |
| `Alt+O` | Files panel | Terminal tab |
| `Alt+S` | Send to the focused tile's session; in a tab with a resident, paste the table's reference into its prompt | Focused tile, or a resident's tab |

`Plus` and `Minus` are matched by the character your layout produces, not by
`Shift`, so `Alt++` and `Alt+=` are the same chord.

## The leader

`Ctrl+Shift+Space` is discovery, not the daily path. It is taken before the key
reaches the terminal, so it works inside a focused tile, and it opens the
keybindings panel. While the panel is open, a bare key runs its action:
leader then `B` opens the Beads tab, leader then `/` searches sessions, leader
then `K` turns keys off. Typing filters the list and `Enter` runs the current
row. The leader and `Alt+K` close the panel again.

A chord that fires shows its keys at the foot of the workspace for a moment, so
you can see what was taken.

## Outside a terminal

- `/` focuses the search of the view in front: sessions, Beads.
- `?` shows the keyboard help.
- `Escape` closes the topmost open surface: Peek, a menu, the keybindings panel,
  the Send drawer. It reaches the terminal only when nothing is open, or when
  typed into Peek's own terminal.

## Turning it off

The **Keys on** button at the right of the tab bar reads **Keys off** when
interception is off. It is per device. Off means CHROTE takes nothing, and every
key goes to the terminal.

> [!NOTE]
> Some desktop environments and browsers claim `Alt` combinations before a web
> page sees them. If a chord does nothing, the system took it; the leader and
> the keybindings panel still reach every action.
