# Changelog

All notable changes to CHROTE will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Added the CHROTE Services platform surface for selected `/srv` services.
- Added Go server-side service registry and proxy routes under `/api/services`
  for TTS Gateway and Context Citadel.
- Added a Services dashboard tab with a TTS console for health, queue/messages,
  playback, backend/voice controls, and enqueue actions.
- Added a Context Citadel operator console for document list/read/edit/save,
  Git-backed history, and grounded ask responses with source paths.
- Added backend and frontend tests for Services route boundaries, token
  redaction, queue rendering, enqueue flow, playback links, Context document
  flows, and missing-token/upstream error states.
- Added product requirements for the Services view, beginning with TTS Gateway
  and Context Citadel operator consoles.

### Changed

- Consolidated the product source of truth in `PRD.md`, separating the current
  durable cockpit, Services Platform V1, and later meta-harness/Agent Teams
  roadmap work.
- Clarified component ownership so CHROTE wraps `/srv` services through
  server-side adapters instead of exposing service credentials to the browser.
- Updated the embedded dashboard assets after building the Services UI.
- Gave Context Citadel ask requests a longer CHROTE proxy timeout than fast service
  health/list/read routes, matching the LLM-backed route latency.

## [0.2.0] - 2025-01-23

### Added
- **ChroteChat**: Dual-channel messaging system (Mail + Nudge) for communicating with AI agents
  - Mail channel for async messages with full message history
  - Nudge channel for quick pings to running sessions
  - Channel selector with unread indicators
  - Message input with recipient selection
  - Collapsible sidebar for conversation threads
  - Mobile-optimized UI with touch-friendly targets
- Long-press support for session context menu on mobile
- Responsive improvements for top bar and terminal view
- Sidebar toggle button in ChroteChat input area
- Protected `chrote-chat` session from "nuke all" operation

### Fixed
- WebSocket proxy for terminal connections
- WebSocket subprotocol negotiation for ttyd
- Tab switching causing terminal session reloads
- Crew worker nudge target resolution
- Header controls collapsing out of view on narrow windows
- Input box hiding on narrow windows
- Sidebar toggle visibility in placeholder view

### Documentation
- Added CI/CD guide for solo devs with personal cloud services
- Added Gas Town Formulas operator guide
- Added operator workflows and test strategy documentation
- Added comprehensive Gas Town characters documentation
- Updated README with ChroteChat features

## [0.1.0] - 2025-01-15

### Added
- Initial public release of CHROTE
- **Dashboard**: React web interface for monitoring and control
  - Dual terminal workspaces with xterm.js
  - Session panel with drag-and-drop session binding
  - Native file browser for /code and /vault directories
  - Beads issue tracking with Kanban/Triage/Insights views
  - Theme support (Matrix, Dark, Gastown)
  - Ambient music player with built-in tracks
  - Crew companions (cosmetic)
- **Go Backend**: Single binary server
  - Embedded React dashboard
  - tmux session management API
  - ttyd WebSocket proxy
  - File browser API with security restrictions
  - Beads integration
- **Infrastructure**
  - systemd services for chrote-server and ttyd
  - Tailscale integration for secure remote access
  - WSL2 optimized configuration
- Official CHROTE soundtrack

### Security
- Non-root chrote user with no sudo access
- File API restricted to /code and /vault paths only
- Dedicated tmux socket directory

[0.2.0]: https://github.com/user/chrote/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/user/chrote/releases/tag/v0.1.0
