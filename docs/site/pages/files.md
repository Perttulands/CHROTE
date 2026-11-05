---
title: Files
nav: Files
summary: Find, read, edit, compare and hand over files under the configured roots, beside the terminal or as a tab of its own.
sources: PRD.md, docs/journeys.md, SECURITY.md
---

# Files

Files is one panel in two places. Beside a terminal (`Alt+O`) it is a sidecar
whose open file hangs off its edge. As the **Files** tab it is the tab's column,
with the file filling the rest. Both browse, find by name, read, edit, compare,
rename and send files.

## Where it opens

The Files tab opens on the host's **projects**: the workspaces the server
finds (folders where live sessions run, configured Beads projects, and git roots
or Beads stores a few levels under the configured roots), grouped by the folder
they live in, with their live sessions and open Beads. Below them are the roots,
for everything else. Inside a project, find looks only there, and the project
lands on its sessions, key documents and README.

A link of the form `/?files=<path>` opens the Files view at that path, which is
handy in a note to yourself or from an agent's output.

## Reading

| File | Shown as |
| --- | --- |
| Markdown | Rendered in the theme, prose set for reading |
| Code, text, JSON | A viewer suited to it |
| Images | Fitted, never upscaled; a click opens the image glance |
| Video and audio | The browser's own controls; a format it cannot play is offered as a download |
| PDF | The browser's viewer |
| GLB models | Inline, with drag to orbit, wheel or pinch to zoom, and Reset view |

## Changing things

- **Edit** a file in place and save. The editor is shared with Brain and Agents.
- **Compare** a file against its committed state in git.
- From a row's menu: **Open**, **Download**, **Rename**, **Copy path**, **Copy
  relative path**, **Open parent folder**, **Delete**; from the background:
  **New file**, **New folder**, **Upload**, **Refresh**.
- Uploads stream to disk and take as long as the link needs, up to
  `CHROTE_MAX_UPLOAD_BYTES` (64 MiB by default).
- In the tree and in search results, Up and Down move between rows and open
  files; Down from Find files enters the list.

Hand a file or a diff to an agent with **Send to Session**; the reference is its
path.

## Boundaries

- Every path must resolve under `CHROTE_ROOTS` **after** symlinks are resolved,
  for reads and writes alike. `CHROTE_ROOTS` is a comma-separated list.
- Inside the roots, Unix permissions decide. A path the service user cannot read
  is reported as a permission error, not hidden. There is no second file policy.
- The roots bound CHROTE's file API only. Programs in tmux keep their own Unix
  user's access.

To change the roots on an installed host, re-run the installer with
`--workspace`, or set `CHROTE_ROOTS` in `secrets.env` and restart; see
[Configuration](configuration.md).
