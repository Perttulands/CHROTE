# Changelog

Notable user-facing changes to CHROTE are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
CHROTE v2 remains alpha: contracts and installation may change before a stable
release.

## [Unreleased]

### Added

- Three independent durable terminal workspaces.
- Unified default-closed Sessions/Files sidecars with Peek and explicit
  attached-window navigation.
- Files workbench and terminal-companion file sidecar.
- Session locking that puts a durable agent under its own systemd user unit.
  Supervision, restart, and boot recovery belong to systemd, so a locked agent is
  unaffected by CHROTE restarting, crashing, or being upgraded. Reported health
  is the unit's state plus a launcher receipt proving the expected agent resumed
  the expected work. Unlocking stops the supervision and leaves the agent
  running.
- Session Bank with typed workload descriptors and workload-aware recovery.
- Scheduled tasks and Server health/history cockpit views.
- Documentation source-truth index and contract lint.

### Experimental

- Unreleased file-backed Formations, missions, typed ports, gates, personas, run
  ledgers, controlled executors, resume, and escalation work on `main`.
- Unreleased `archon` CLI and mission-room projection work over the same
  experimental storage contracts.

### Changed

- Filesystem operations now resolve and constrain symlink mutations under
  configured roots.
- Terminal drag/drop, assignment, iframe lifecycle, and layout persistence were
  hardened for dense multi-window use.
- Session rows now Peek without changing assignment metadata; location chips
  perform explicit attached-window navigation.
- Bulk session destruction moved to advanced recovery settings.
- Optional services and workspaces degrade explicitly instead of silently
  fabricating data.

### Fixed

- Locked-agent units now track the agent's exact pane lifetime: the pane starts
  through a fixed launcher, typed config becomes an argv without `send-keys`,
  and agent exit makes the supervising unit restart the same native session.
- Locked-agent config and receipt files now occupy separately provisioned
  ownership domains; receipt writes fail closed and reads validate mode, owner,
  regular-file type, and symlink boundaries.
- Locked-agent health now requires a live observed agent process and a receipt
  bound to the current systemd invocation, pane, PID start identity, and unit
  start time; the unit does not report ready until that proof is published.
- Cross-user locking now uses a shipped, validated sudoers grant and one absolute
  root-owned helper. Startup probes each configured user's real manager and the
  dashboard disables locking when that capability is unavailable.
- Fresh terminal sidecars persist their default-closed state across reloads.
- `/` opens Sessions for the active closed sidecar and focuses search.
- Workload recovery records and restores supported agent, shell, and server
  identities without claiming arbitrary shell-state resurrection.
- Hidden keep-alive views no longer steal Escape from the visible interaction
  surface.

### Security

- Removed the revoked access-token/authentication experiment; CHROTE retains its
  documented localhost/private-network trust model.
- Tightened file-root, symlink, terminal-origin, command-argument, gate-output,
  and recovery-descriptor boundaries.
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
