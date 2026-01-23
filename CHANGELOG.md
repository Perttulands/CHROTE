# Changelog

All notable changes to CHROTE will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
