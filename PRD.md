# CHROTE product requirements

[`VISION.md`](VISION.md) explains why CHROTE exists. This document defines the durable product contract. Beads owns planned work, dependencies, status, and outstanding decisions.

## Operator and environment

CHROTE is a browser-based agentic IDE for one trusted operator managing command-line work on one host. [`docs/journeys.md`](docs/journeys.md) states the seven journeys that are this product's use cases, and every surface answers to one of them.

The host runs tmux, the CHROTE server, and the resources exposed through CHROTE. Remote access uses an operator-controlled private network such as Tailscale. CHROTE does not provide accounts, login, roles, or tenant isolation.

## Terminal workspace

- CHROTE discovers sessions from configured tmux sockets and can create, attach, display, and explicitly delete sessions.
- Terminal tabs provide independent layouts. A layout can show one to four terminal windows.
- The Sessions sidecar supports discovery, Peek, navigation, attachment, and creation without silently changing a window's assigned session.
- Creating a session offers the harnesses and folders named in host configuration, so a session starts in the intended directory as the intended Unix user with its harness already running.
- Browser disconnects, tab closure, and CHROTE restarts leave existing tmux sessions running.
- Cleanup may remove only the exact session the operator authorized or the exact test-owned or failed-creation-owned session being cleaned up.
- Any command-line program can run in a CHROTE terminal. Core terminal behavior does not depend on a particular agent harness.
- Painting a selection in a terminal copies it to the system clipboard. Under tmux mouse mode that is the browser's own selection gesture; tmux's copy-mode selection from a plain drag is separate and never reaches the system clipboard.
- A leader chord reaches CHROTE's own actions from anywhere, including a focused terminal, shows what it offers while it waits, and is disabled entirely by a device-local toggle.
- Send to Session is the one path for handing work to an agent: it draws in as a layer over the right edge of the view it was opened from, and carries a one-line reference to the object being handed over.
- Send to Session reports delivery to tmux. It does not claim that the process consumed or understood the message.

## Files

- GLB models open inline with drag-to-orbit, wheel/pinch zoom and Reset view.
- In the file tree and sidebar search results, Up/Down moves between visible rows and opens files; Down from Find files enters the list.

- The Files view and terminal sidecar browse, read, edit, compare, rename, and send files under configured roots.
- Canonical-path checks keep file operations inside those roots after symlink resolution.
- Unix permissions decide what the service identity can access. CHROTE reports permission failures plainly and does not add a second file-access policy.
- File state remains in the filesystem. CHROTE does not mirror project contents into its own database.

## Server and settings

- Server shows health, resource readings, runtime events, and bounded operational history.
- Settings owns presentation and explicit operator controls for terminal behavior.
- Layouts, labels, presets, and other device-specific presentation live in browser storage.
- The interface has one active theme. It is host state, authored on the host and served by the server; the browser stores no theme and offers no picker.
- Opening or reloading CHROTE must not replay stale browser settings into live tmux servers. Host-wide tmux changes require an explicit operator action.

## Components

The core product is terminal workspaces, sessions, files, server status, and settings. These first-party components add separate capabilities:

| View | Operator job |
| --- | --- |
| Terminal 1 | First independent tmux workspace |
| Terminal 2 | Second independent tmux workspace |
| Terminal 3 | Third independent tmux workspace |
| Files | Work with files under configured roots |
| Server | Inspect host and CHROTE health |
| Settings | Configure presentation and explicit terminal controls |
| Beads | Inspect and update configured project work stores |
| Scheduled | Send prompts to named tmux sessions on a schedule |
| Brain | Understand the context corpus by subject, follow references, read and correct pages |
| Agents | Read the context a harness would get for any folder on the host |

Beads, Brain and Agents each host a resident agent, the Clerk, the Librarian and the tender, as a tmux session in a column at the right of the view; the one selected object, the table, is carried from view to view.

Brain is search first. The search asks the brain service's ranked search as
the operator types and lists each page by title, kind, summary, matched words
and path, topics first; a scope control widens it to Self or every collection.
Opening a result reads the page wide beside the list, which stays; Up and Down
move through the results and Escape closes the page, not the list. Without a
query, Brain offers Subjects and Map modes over the same corpus. Subjects expands
one group at a time. Map is a night sky for getting a feel of what the brain
knows and whether it looks in order: each subject is a star system with its
topic at the centre, linked pages in orbit, its evidence as nebula and unlinked
pages drifting past its edge; subjects that cite each other form
constellations. Records are drawn with the subject they were filed for. Systems
keep their places as the corpus grows and when filters change. Pointing at
empty sky names the pages nearby; selecting a page reads it beside the sky,
traces its links and signposts those that leave the screen. A search can be
shown on the map, lighting what it found. Played, the time scrubber replays the
brain's growth, new pages flaring as they arrive. Motion is reduced when the
system asks for it. A subject selector gives direct access to regions whose
labels are outside the current view. Collection,
facet and kind filters narrow the map; cross-subject navigation follows explicit
links. Generated indexes and agent instructions are excluded. Refresh and tab
entry re-read the tree, including moved pages. The existing reader, editor,
history and Librarian remain available. Switching modes preserves filters and
selection, with a separate camera for each mode. Four device-local view slots
store mode, camera, filters, selection and time setting against current content.
Click an empty slot to save or an occupied slot to recall; its context menu offers
Rename, Replace and Clear. The time slider filters page arrival dates; saved
views do not store historical copies of page content.

Components remain separated in the codebase and fail independently. A missing Beads workspace or unconfigured context corpus must not prevent terminal work.

Beads browsing shows remembered successful work immediately and fills a first
host overview project by project. Visible work updates automatically while
preserving query, selection, graph position, table trail and drafts. Each store
reports whether its data is current, refreshing, stale or unavailable; incomplete
host data must not imply there is no work. Closed work loads when requested.

CHROTE exposes terminal sessions and delivers operator input. The programs
running in those sessions own their work, coordination and completion rules.

## State ownership

- tmux owns live sessions and the processes inside them.
- The filesystem owns files.
- Each project's Beads store owns its work state.
- Host configuration owns schedules, service adapters, executable paths, tmux socket mappings, the active theme, and the harnesses and folders the launcher offers.
- The browser owns device-local presentation.
- CHROTE stores only state required for behavior it uniquely owns.

## Trust and deployment

- The default server binding is loopback-only.
- Anyone who can reach CHROTE is inside its trusted operator boundary and receives terminal-grade capabilities.
- Private networking and HTTPS protect remote access. CORS is not authentication.
- Tokens and service credentials remain in private server configuration. Browser code calls CHROTE-owned routes rather than receiving those credentials.
- Broad access within configured roots is intentional. CHROTE may add an explicit access grant but must not silently narrow ownership, modes, ACLs, or roots.

See [`SECURITY.md`](SECURITY.md) for the public trust contract.

## Cross-device behavior

Desktop is the full workspace. Phones and tablets must preserve the essential loop: see sessions, inspect output, answer prompts, send messages, and perform bounded session actions. Complex multi-window arrangement may remain desktop-first.

Device-local layouts may differ. Shared sessions, files, Beads, schedules, and services still refer to the same host resources.

## Non-goals

- Multi-user collaboration, tenant isolation, or a hosted SaaS control plane.
- Replacing tmux, Git, Beads, the filesystem, or an agent's command-line interface.
- A third-party plugin marketplace.
- Reconstructing ordinary processes after a host reboot or process death.
- A second access-control system layered over configured roots and Unix permissions.
- Keeping project plans, roadmap, or status in product documentation.

## Product acceptance

- The React dashboard and Go server build reproducibly from one source tree.
- The server exposes a healthy `/api/health` endpoint on a supported installation.
- Existing tmux work survives browser disconnects and CHROTE service restarts.
- Optional components degrade without breaking the terminal core.
- File operations remain inside configured roots and preserve Unix permission errors.
- Public source and documentation remain host-neutral and contain no credentials.
