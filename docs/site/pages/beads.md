---
title: Beads
nav: Beads
summary: Every Bead in every store on the host in one table, sliced by search, project, status and days, with each Bead's family beside it.
sources: PRD.md, DESIGN-SYSTEM.md, docs/journeys.md, docs/troubleshooting.md
---

# Beads

[Beads](https://github.com/steveyegge/beads) is an issue tracker that lives
inside each project, driven by the `bd` command. The Beads tab reads every store
it can find on the host and shows them as one table. Writing Beads stays with
`bd` and with agents; CHROTE reads, slices, files a quick report, and hands work
over.

## Requirements

- `bd` on the service's `PATH`, or an absolute path in `CHROTE_BD_COMMAND`.
- Stores to find. CHROTE reads the projects named in `CHROTE_BEADS_WORKSPACES`
  (comma-separated; the installer names your workspace), and discovers any
  folder with a `.beads` store up to three levels under the configured roots
  and the launchable users' homes, or where a live session runs. Add a store
  nested deeper under **Settings → Beads projects**.

Without `bd` the tab says so, and nothing else in CHROTE is affected.

## The table

One table of every Bead, shown at once from each store's last read. A store
written to since is re-read in the background; its row in the rail shows the
age of what you are looking at, and the footer counts stores still being read.
Stale is stated, never hidden behind a spinner.

- **The strip** above counts Beads created and closed per day over the last
  sixty days, with the open count as a faint area behind. Drag across it to
  keep only those days.
- **The rail** filters by project and status. Each row toggles; its name keeps
  it alone.
- **Search** (`/`) finds ids, titles and the text of each Bead.
- **Columns sort.** Sort by Idle or Size to bring the oldest or largest open
  work to the top.
- **Type** is written as its word in its colour: bug, feature, chore, decision,
  epic, task. Finished Beads are dimmed end to end.

## A Bead's family

Select a row and the **work graph** opens beside the table, with the Bead's
parent, children and the blocking edges to and from other stores. Finished
cards are dimmed and blocked cards dashed; card borders carry their project's
colour, named in the legend. `Enter` or a double-click opens the Bead's card on
the table. `Escape` lets the selection go.

**Map** shows the same slice as the Brain's sky: each epic a star system, open
work in orbit, finished work as glow.

**Copy the view's link** and it opens the same slice in a fresh tab.

## Handing a Bead over

The Clerk, the agent that keeps the register, lives behind the spine at the
right of the tab when the host configures one. `Alt+S` pastes the selected
Bead's reference into the Clerk's prompt; you write the rest and press `Enter`
there. Send to Session carries the same reference to any other session.

From any tab, `Alt+B` toggles a Beads column beside the workspace, and a Bead id
printed in terminal output opens on the table beside the session that named it.

## Filing a quick report

`Alt+R`, from anywhere, files a bug or feature as a Bead in a store the tab
knows, with an optional line saying which tab and session you were in. See
[Send to Session](send-to-session.md#report-a-problem-from-anywhere).

## Cache

The last read of each store is kept on disk under `$XDG_CACHE_HOME/chrote`, so a
restart serves the table at once. It is disposable: delete it and the next visit
reads the stores again.
