# Captures the on-board live terminals (03-formations.js: openTerm, feed, MOCK #2 → real
# ttyd/websocket stream). S0 canvas terminals are watch/focus surfaces. Driving a live session is via
# `archon agent attach`; in-popup keystroke forwarding is deferred. The roster + liveness come from
# the existing Oracle/tmux session source.

Feature: Live agent terminals on the canvas
  As a human supervising agents
  I need to open a live terminal for any agent right on the board
  So that I can watch a session without manual socket/session hunting

  Background:
    Given a board with a staffed formation and a live agent roster

  @ui
  Scenario: Open a terminal from an agent or a slot
    When I click a roster agent without dragging, or choose "Open terminal" on an agent or filled slot
    Then a terminal popup opens for that agent on the board

  @ui
  Scenario: Terminals live in world space
    When the canvas pans or zooms
    Then open terminals pan and zoom with the world
    And a terminal can be dragged by its header and resized from its corner

  @ui
  Scenario: Re-opening an agent focuses its existing terminal
    Given a terminal for "scout" is open
    When I open "scout" again
    Then the existing terminal is focused and highlighted, not duplicated

    @cli @ui
    Scenario: The terminal streams the real session, not a mock
      When a terminal is open for a live agent
      Then it streams that agent's session output (ttyd/websocket)
      And it reflects the agent's state (attached/idle/busy/dead) from the session source

    @ui
    Scenario: Canvas terminal input is deferred to explicit attach
      When a terminal popup is open on the canvas
      Then S0 requires read-only streaming and focus
      But keyboard takeover, paste, and prompt submission are done through "archon agent attach"

  @ui
  Scenario: A terminal for a dead or missing session is shown as such
    Given an agent whose session has exited
    When I open its terminal
    Then it indicates the session is not live rather than appearing connected
    # Fail loud: never present a dead session as attached.

  @ui @security
  Scenario: Run-bound Peek never shows newer work as an old attempt
    Given an old slot dispatch records binding, target lease, opaque target, and frozen fingerprint
    When that exact dispatch still owns active unmatched occupancy or its terminal hold on the unchanged pane
    Then run-bound Peek may stream it read-only with the exact attempt identity
    When occupancy becomes a release receipt, the pane is quarantined, or a later run reuses the target
    Then the old attempt shows captured history, unavailable, or "pane_moved_on"
    And it never labels current live bytes as evidence from the old run
    And "Open current session" is a separate explicitly non-run action

  @ui
  Scenario: Closing a terminal never disrupts the underlying session
    When I close a terminal popup
    Then only the popup closes
    And the agent's tmux session keeps running (CHROTE golden rule)
