# Captures the infinite-canvas behaviors (03-formations.js: pan/zoom/fitView, dragCard/dragGate/
# dragMission, obstacle-aware wire routing, undo stack, on-board terminals). Node positions and
# wire lanes are LAYOUT (sidecar), never structure — deleting layout loses positions, never the graph.

Feature: The formations canvas — pan, zoom, arrange, and undo
  As a human reading a complex mission graph
  I need a legible infinite canvas I can navigate and tidy
  So that the shape of the work is digestible without altering its structure

  Background:
    Given the always-available Formations tab is open
    And a board with several formations, a gate, and a mission

  # ── Navigation ──────────────────────────────────────────────────────────────

  @ui
  Scenario: Pan the canvas by dragging empty space
    When I drag on empty canvas
    Then the world pans and the cursor shows grabbing
    And dragging a node instead moves the node, not the canvas

  @ui
  Scenario: Zoom with the wheel around the cursor, and with the zoom controls
    When I wheel up over a point
    Then the canvas zooms in centered on that point
    And the zoom level indicator updates
    And the +, −, and FIT controls zoom and frame the content

  @ui
  Scenario: Fit frames all nodes
    When I click FIT
    Then all nodes are framed within the viewport with padding
    And wires are redrawn correctly after the transform

  # ── Arranging (layout, not structure) ───────────────────────────────────────

  @ui @file @layout
  Scenario: Moving a node writes only to the layout sidecar
    When I drag a formation to a new position
    Then its x/y is saved in the layout sidecar
    And the board definition file is unchanged
    And "archon formation inspect" shows no structural diff

  @ui @cli @file @layout
  Scenario: Only a new element receives automatic placement
    Given the board has a hand-arranged persisted layout
    When the UI or Archon creates one connected or unconnected element
    Then only that new element receives connection-aware placement when it has a neighbor
    And otherwise only that new element receives bounded free-space grid placement
    And every existing node coordinate and hand-routed lane remains byte-for-byte unchanged

  @ui @cli @file @layout
  Scenario: Full arrangement is explicit
    Given the board has a hand-arranged persisted layout
    When I open, validate, save, run, replay, reconnect, or add a connection
    Then no existing element is rearranged
    When I explicitly invoke Arrange in the UI or the Archon arrange verb
    Then the shared deterministic arrangement operation may update the full layout sidecar

  @ui @layout
  Scenario: Wires route around cards automatically, and re-route as nodes move
    Given two nodes with a card between them
    Then the connecting wire curves around the obstacle rather than under it
    And while a node is actively dragged its wires do not fight the drag

  @file @layout
  Scenario: Deleting the layout sidecar loses positions but never the graph
    Given a schema-1 board may contain a legacy inline verification
    When the layout sidecar is removed
    Then nodes may render at deterministic fallback positions without writing them
    And open, render, and save do not persist fallback positions for existing nodes
    And only a later direct move, new-element creation, or explicit Arrange may write layout coordinates
    And every node, port, edge, brief, gate, and legacy verification still exists unchanged

  # ── Undo ────────────────────────────────────────────────────────────────────

    @ui
    Scenario Outline: Every mutating gesture is undoable
      When I "<action>"
      And I press undo
      Then the board returns to its previous state
    Examples:
      | action                          |
      | create a formation              |
      | assign an agent to a slot       |
      | wire two nodes                  |
      | delete a gate                   |
        | hand-route a wire               |
        | rename a formation              |

    @ui
    Scenario: Undo does not fire while typing in a field
      Given focus is in a text input or textarea
      When I press Ctrl/Cmd+Z
      Then the field's own text undo applies, not a board undo

    @ui
    Scenario: Undo covers board and layout mutations, not run output or terminal window state
      When a run produces output or a terminal popup is opened
      Then those changes are not pushed onto the board undo stack
      But structural board mutations and layout mutations remain undoable

  # ── On-board terminals ──────────────────────────────────────────────────────

  @ui
  Scenario: An agent terminal opens on the canvas and lives in world space
    When I click an agent (without dragging) or choose "Open terminal"
    Then a terminal popup opens on the board
    And it pans and zooms with the world
    And it can be dragged and resized
    And opening the same agent again focuses the existing terminal rather than duplicating it
    # (The live terminal stream is a backend integration point — see terminals.feature.)
