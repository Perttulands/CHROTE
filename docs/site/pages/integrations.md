---
title: Libraries and residents
nav: Libraries and residents
summary: Connect the context corpus Brain reads, and give each tab its resident agent.
sources: docs/installation.md, ARCHITECTURE.md, PRD.md, src/internal/api/residents.go
---

# Libraries and residents

Both are optional. Without them, Brain says no library is configured and each
resident's drawer says it is not configured; nothing else is affected.

## The Library

The corpus Brain shows is a Markdown tree under git. Its top-level directories
are its **shelves**. CHROTE derives subjects from paths rather than keeping a
taxonomy of its own: a record under `<collection>/<producer>/<subject>/` joins
the subject of that name. Generated indexes and agent instructions are left out.

```bash
CHROTE_LIBRARY_ROOT=/absolute/path/to/corpus
CHROTE_LIBRARY_AUTHOR="Example Operator <operator@example.invalid>"
CHROTE_LIBRARIAN_SESSION=librarian
CHROTE_LIBRARY_BEADS=/absolute/path/to/corpus
CHROTE_LIBRARY_SEARCH_URL=http://127.0.0.1:<port>
CHROTE_LIBRARY_LINK_ROOTS=/absolute/path/to/projects:/absolute/path/to/more
```

- **Reading** lists shelves, pages, recent changes and the link graph from the
  tree and its git log. If page history stays empty while the corpus has
  commits, the service account cannot run `git` there; the log says why.
- **Editing** writes one page and commits it as `CHROTE_LIBRARY_AUTHOR`, and
  refuses a page that already carries somebody else's uncommitted change. Every
  path is validated inside the root; CHROTE writes nowhere else in the corpus.
- **Search** forwards the query to `POST <CHROTE_LIBRARY_SEARCH_URL>/v1/library/search`
  and keeps no index of its own. The search service is yours to run.
- **Project READMEs** can appear on the `projects/` shelf as symlinks to each
  project's own README, never copies. They are read-only in Brain.
  `CHROTE_LIBRARY_LINK_ROOTS` limits where such a link may point.

A root that is set but is not a directory stops the server at startup.

## Residents

A resident is a tmux session that lives in a tab, behind a 28 px spine at its
right edge. Opened, it is a drawer with a live terminal that slides over the
content, never over the table, so the object you are handing over stays in
sight. The drawer's menu offers Peek, Send, Launch and Restart. When the session
is absent, the drawer offers **Launch** in the resident's folder. Open or closed
is remembered per tab, and the session persists, so you come back to the agent
mid-answer.

| Tab | Resident | Configured by |
| --- | --- | --- |
| Brain | Librarian | `CHROTE_LIBRARIAN_SESSION`, `CHROTE_LIBRARY_ROOT` as its folder, `CHROTE_LIBRARY_BEADS` |
| Agents | Tender | `CHROTE_TENDER_SESSION`, `CHROTE_TENDER_FOLDER`, `CHROTE_TENDER_BEADS` |
| Beads | Clerk | `CHROTE_CLERK_SESSION`, `CHROTE_CLERK_FOLDER`, `CHROTE_CLERK_BEADS` |
| Services | Service agent | `resident` in [`services.json`](services.md#configure-it) |

Each resident is a session name, the folder its launcher starts in, and the
Beads project whose open Beads are its proposals. The server does not start
residents; you do, from the drawer. What each resident is told to do, its
charter, lives on your host in that folder, outside CHROTE.

```bash
CHROTE_TENDER_SESSION=tender
CHROTE_TENDER_FOLDER=/absolute/path/to/tender
CHROTE_TENDER_BEADS=/absolute/path/to/store
CHROTE_CLERK_SESSION=clerk
CHROTE_CLERK_FOLDER=/absolute/path/to/clerk
CHROTE_CLERK_BEADS=/absolute/path/to/store
```

`GET /api/residents` reports all four, one entry per tab whether or not anything
is configured, as `[{ tab, label, session, folder, beads }]`. Brain's entry
keeps its route name, `library`.

## Beads

The Beads tab needs `bd`. If it is not on the service's `PATH`, set
`CHROTE_BD_COMMAND` to its absolute path. `bv`, the graph-aware Beads TUI, is
optional and runs in a terminal like anything else. See [Beads](beads.md).
