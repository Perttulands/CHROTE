---
title: Brain
nav: Brain
summary: The context corpus drawn as a night sky of subjects, with search, a reader for prose, and edits committed as you.
sources: PRD.md, ARCHITECTURE.md, docs/installation.md, DESIGN-SYSTEM.md
---

# Brain

Brain shows a **context corpus**: a Markdown tree under git that you and your
agents keep as shared memory. Its configuration and routes still carry the
older name, the Library: `CHROTE_LIBRARY_*` variables and `/api/library`. No
corpus is bundled; without one, the tab says no library is configured.

## The sky

The sky is for getting a feel of what the corpus knows and whether it looks in
order.

- Each **subject** is a star system with its topic at the centre, linked pages
  in orbit, its evidence as nebula, and unlinked pages drifting past its edge.
  Subjects that cite each other form constellations.
- **Brightness** is importance, **haze** is evidence, **dim** is finished.
  Circles mark topics and notes, squares research, diamonds sources.
- **The rail** lists collections; hiding one takes it off the sky. Systems keep
  their places as the corpus grows and when a collection is hidden.
- **Time** runs along the foot. Play it to replay the corpus's growth, pages
  flaring as they arrive; drag it to see the corpus as it stood on a day.
- A system's name takes the camera to it; **Fit** takes in the whole.
- Motion is reduced when your system asks for it. On a phone, the same subjects
  and pages are a list.

## Search

The search along the top asks the configured search service's ranked search over
every collection as you type, and lists each page by title, kind, summary and
matched words, while the sky lights what it found. Pointing at a result lights
its star. Up and Down move through results; `Escape` clears the search before it
closes the page. Without `CHROTE_LIBRARY_SEARCH_URL`, search says it is not
configured.

## The reader

A star opens its page in a reader beside the sky, and the sky traces the page's
links. The reader gives prose room: a one-line head with the page's subject and
when and by whom it last changed, History folded until asked for, and at the end
what the page is related to. **Edit**, **Send** and **Close** stay on the head.

**Edit** saves one page and commits it to the corpus's git history as the
configured author (`CHROTE_LIBRARY_AUTHOR`). A page that already carries
somebody else's uncommitted change is refused rather than overwritten, and the
refusal says why.

## The Librarian

The Librarian is the resident agent of this tab: a tmux session behind the spine
at the right edge, opened over the reader. `Alt+S` pastes the page you are in
into the Librarian's prompt; `Alt+Enter` focuses the drawer. A shelf's menu
sends the whole shelf.

## Setting it up

| Variable | What it names |
| --- | --- |
| `CHROTE_LIBRARY_ROOT` | The corpus directory, a git repository of Markdown. Its top-level directories are its shelves |
| `CHROTE_LIBRARY_AUTHOR` | `Name <email>` your edits are committed as |
| `CHROTE_LIBRARIAN_SESSION` | The Librarian's tmux session |
| `CHROTE_LIBRARY_BEADS` | The Beads project the Librarian works from |
| `CHROTE_LIBRARY_SEARCH_URL` | A search service answering `POST <url>/v1/library/search` |
| `CHROTE_LIBRARY_LINK_ROOTS` | Colon-separated directories that `projects/` symlinks may point into |

A root that is set but is not a directory stops the server at startup, with the
reason in its log. Details: [Libraries and residents](integrations.md#the-library).

`Alt+L` opens Brain from anywhere.
