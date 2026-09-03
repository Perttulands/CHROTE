# Changelog

Notable user-facing changes to CHROTE are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
CHROTE v2 remains alpha: contracts and installation may change before a stable
release.

## [Unreleased]

### Added

- Three independent tmux-backed terminal workspaces.
- Unified default-closed Sessions/Files sidecars with Peek and explicit
  attached-window navigation.
- Files workbench and terminal-companion file sidecar.
- Scheduled tasks and Server health/history cockpit views.
- Documentation source-truth index and contract lint.
- One host-authored interface theme, served by the server at `GET /api/theme`
  with its art at `GET /api/theme/art/{name}`, driving the chrome, the terminal
  palette, the per-user identity colours, and the empty-window art.
- A session launcher in every empty window and behind the Sessions plus: pick a
  harness, a folder, a Unix user and a name, from the harnesses and folders host
  configuration names at `GET /api/launch`. The new session starts in that
  directory with its harness running and binds to the window it was launched
  from.
- JetBrains Mono and a symbol fallback face bundled for chrome and terminal
  alike, so no request for a font leaves the host.
- Agent events: a launched harness reports through its own completion hook,
  the `chrote-agent-event` script installed beside the server, to
  `POST /api/agent/event`; the session list carries the last report as
  `lastEvent` until `POST /api/agent/event/seen`. The launcher installs the
  hook for Claude Code and Codex while its **Notify on completion** setting is
  on.

### Changed

- Files now exposes everything under `CHROTE_ROOTS` that the service identity's
  Unix permissions allow, while canonical paths remain inside configured roots.
- Terminal drag/drop, assignment, terminal lifetime, and layout persistence were
  hardened for dense multi-window use.
- Session rows now Peek without changing assignment metadata; location chips
  perform explicit attached-window navigation.
- Bulk session destruction moved to advanced Settings.
- Optional services and workspaces degrade explicitly instead of silently
  fabricating data.
- The interface is monochrome except where colour carries meaning: errors and
  danger, focus and the primary action, the Claude Code mark, and Unix-user
  identity taken from the theme.
- Tiles and session rows show the running agent as its product mark instead of a
  `foreground:` box, and a session name that must shrink keeps the tail after
  its last hyphen rather than the head.
- A focused tile changes border colour only. The border no longer changes width,
  so focusing a tile moves nothing.

### Removed

- Extracted the unreleased Formations and Archon experiment, including its
  history, to `chrote-agent-formations`.
- Removed the Agents tab; agent work remains visible through tmux and native
  harness state.
- Retired the unreleased session-locking and Persistent Agents capability,
  including CHROTE-owned agent units and reboot-recovery claims. CHROTE preserves
  external tmux work across its own lifecycle but does not supervise ordinary
  sessions.
- Removed the tmux appearance API and the Settings theme, tmux-colour and
  per-user colour pickers. A device-local choice no longer rewrites host-global
  tmux state; the host applies the theme and CHROTE serves it.
- Removed the scanline overlay, the tile and tab glows, and decorative motion.
- Removed the background art from the server binary. Art belongs to the host
  theme and is served from the theme directory.

### Fixed

- Fresh terminal sidecars persist their default-closed state across reloads.
- `/` opens Sessions for the active closed sidecar and focuses search.
- Hidden keep-alive views no longer steal Escape from the visible interaction
  surface.

### Security

- Removed the revoked access-token/authentication experiment; CHROTE retains its
  documented localhost/private-network trust model.
- Kept canonical file-root and symlink containment while removing the extra
  sensitive-path classifier.
- Release builds are being moved to a patched Go baseline with blocking source
  and binary vulnerability scans before the next tagged alpha.

## [2.0.0-alpha.1] - 2026-05-20

### Added

- Started the CHROTE v2 architecture for a host-owned browser cockpit.
- Introduced the Go API/embedded dashboard split and the initial durable
  terminal, Files, Beads, Agents, Services, Settings, and Help surfaces.
- Preserved CHROTE v1 on its tagged legacy line.

## [1.0.0] - 2026-03-07

### Added

- Tagged the legacy v1 terminal cockpit before the v2 architecture transition.

[Unreleased]: https://github.com/Perttulands/CHROTE/compare/v2.0.0-alpha.1...HEAD
[2.0.0-alpha.1]: https://github.com/Perttulands/CHROTE/releases/tag/v2.0.0-alpha.1
[1.0.0]: https://github.com/Perttulands/CHROTE/releases/tag/v1.0.0
