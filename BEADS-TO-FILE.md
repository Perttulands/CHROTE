# Beads to file for branch `lands/gracious-pasteur-mrwxis`

> **Temporary handoff. Delete this file in the same commit that files these
> Beads.** It exists only because this branch was built in a cloud session
> without `bd`; tracker state belongs in the `chrote-` store, not in the tree.

File these in the `chrote-` store, then close the ones marked done with the
commits named.

## Close existing Beads

| Bead | Close reason |
| --- | --- |
| `chrote-6bz3` The squeeze counts the rail as content | Removed with the squeeze itself: every resident is a drawer (44aafdb). |
| `chrote-jf5l.4` Beads tab: the title gets the room | Clerk column gone (44aafdb); detail only while selected; type column; narrow tables drop Updated and Size (9e9814b). |
| `chrote-jf5l.5` Files tab: the operator's files, not the filesystem | Files tab is the Files panel as a tab: pins above the tree, one Refresh, no DIR/TXT badges, the file beside the tree (153bc78). The "missing-file state" is the panel viewer's own read-failure message. |
| `chrote-5grx.24` Remove the dedicated Files tab once the side panel has proven itself | Resolved differently, on the operator's direction (this session): the tab stays, but it is now the panel itself, so the duplicate code is gone (153bc78). |

## New Beads, already done (file and close)

1. **task**, parent `chrote-jf5l`. *Every resident is a drawer behind a spine.*
   The Clerk and the tender rest behind the Librarian's 28px spine and open over
   the content; the column mode, the squeeze and the menu's Collapse row are gone.
   The open drawer stops at the table so a card being handed over stays visible.
   Close: 44aafdb, 9e9814b, and the commit titled "Keep the draft warning
   and the drawer's room past their viewer".
2. **task**, parent `chrote-5grx`. *One SearchField for every search box.*
   Expansion beyond the assigned work, filed as such: the operator named
   search-bar drift. The kit gains `SearchField`; ten inputs (Beads, its
   project rail, Brain, Agents, Files tab, Files and Sessions sidecars, Send,
   flags, keybindings) use it, and their separate styles are gone. Close: aa15f3c.
3. **feature**, parent `chrote-jf5l`. *Beads Map: the slice in the Brain's sky.*
   Expansion, on the operator's request: "the graph adopts the Brain's look &
   feel … seeing all the open beads of a project in a map". Split/Table/Graph
   become List and Map; each epic is a system, open work orbits it and finished
   work is its glow; unfiled work drifts around "Unfiled in <project>". SkyMap
   takes a storage key, system names and words. Close: 9e9814b, f9b7e01.

4. **feature**, parent `chrote-jf5l`. *Files: a portal into the host's projects.*
   Expansion, on the operator's request (about 100 project folders over two
   homes and /srv). The Files tab opens on `GET /api/workspaces` grouped by
   parent folder, live projects first, roots last; a project scopes find
   (`GET /api/files/find?within=`, new server surface) and lands on its
   sessions, key documents and README. Close: the commit titled "Files opens on
   the host's projects".

Review of this branch (Terra, at `ca8de1e`) found four defects in items 3 and 4; all four are
fixed in the commit titled "Files keeps unsaved edits; PDFs, missing pictures
and the Beads map on a phone", with a test that reproduces each: unsaved
drafts lost on switching files, the Beads map collapsing on a phone after a
selection, PDFs no longer shown inline, and a missing picture drawn broken.
Terra's follow-up note led to "Keep the draft warning and the drawer's room
past their viewer": the leave-page warning lifted when the last viewer closed,
and the Clerk's drawer covered the table's first 28px.

## New Beads, open (follow-ups noticed, not done)

5. **task**, parent `chrote-5grx`. *Peek opening test flakes under full-suite load.*
   `tests/peek.spec.ts` "opens at one size, and a pane taller than the room…"
   failed twice in full runs this session (scale 1 and 1.25), passing alone 3/3.
   Likely the same cause as the open `chrote-ezfj` (frame-rate dependency); attach
   there rather than filing anew if it matches.
6. **task**, parent `chrote-jf5l`. *Beads Map foot carries only Fit.*
   The map reuses BrainSky's 52px foot with no time line, so it holds a lone Fit
   word. Either give the map the Brain's time line over created/closed days or
   drop the foot and put Fit in the work head.
7. **decision**, parent `chrote-jf5l`. *Should the Map colour stars by Bead type?*
   Stars take their system's hue, as in the Brain. The type is legible in List
   and in the selected Bead's text, but a bug and a task look alike on the sky.
8. **feature**, parent `chrote-jf5l`. *Files: what changed, across every project.*
   Expansion, proposed and deferred by the operator until the projects portal
   has been used. A Changed view of uncommitted changes in every project,
   newest first, each file opening in the panel's diff. Needs one read-only
   route answering `git status` per workspace through `src/internal/api/git.go`.
