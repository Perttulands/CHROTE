# Captures the prototype's right-click model (03-formations.js: menuAgent, menuFormation,
# menuInputRow, menuOutputRow, menuVerification, menuSlot, menuGate, menuWire, menuMission, the
# board contextmenu, showAssignMenu, openJudgePicker). Principle (D7): EVERY element is
# right-clickable and exposes the useful commands you'd expect there.

Feature: Right-click anything — context menus expose the expected commands everywhere
  As a human inspecting and lightly tweaking a formation graph
  I need every element to offer its sensible actions on right-click
  So that "whatever I expect to work, works" without hunting through panels

  Background:
    Given a board with a mission, formations (solo/peer/flow/orchestrated), a gate, wires, and an agent roster
    And the formation card is a catch-all target, with element-specific menus layered on top

  @ui
  Scenario: Right-clicking empty board canvas offers creation
    When I right-click empty canvas
    Then I can create a Mission, a Solo/Peer/Flow/Orchestrated formation, a Gate, or a plan template
    And the new node is placed at the cursor position

  @ui
  Scenario: Right-clicking a roster agent offers its agent actions
    When I right-click an agent in the roster
    Then I can open its terminal
    # (Discovery/inspection/factory actions for agents live in agents.feature / agent-factory.feature.)

  @ui
  Scenario: Right-clicking a formation (card, header, or body whitespace) offers the full menu
    When I right-click a formation
    Then I can Run, Rename, Add slot/step (non-solo), Add input, Add output,
      Add/Configure/Remove verification, Clear output, Set input, Duplicate, and Delete

  @ui
  Scenario: Right-clicking an input row offers input actions
    When I right-click a formation's input row
    Then I can Edit input, Add input, Remove last input (when more than one), and Run the formation

  @ui
  Scenario: Right-clicking an output row offers output actions
    When I right-click a formation's output row
    Then if there is output I can Open report or Clear output
    And I can always Add output, or Remove last output when more than one

  @ui
  Scenario: Right-clicking the verification band offers verification actions
    When I right-click the verification band
    Then I can Add verification when none exists
    And otherwise Configure or Remove it

  @ui
  Scenario: Right-clicking a slot offers assignment and slot actions
    When I right-click a slot
    Then I can Assign/Change agent (a roster submenu), and when filled Open terminal or Unassign
    And in an orchestrated formation a non-controller slot offers Make controller
    And a non-solo formation offers Add slot/step after and Remove slot

  @ui
  Scenario: Right-clicking a gate offers gate actions
    When I right-click a gate
    Then I can Configure, Duplicate, and Delete it

  @ui
  Scenario: Right-clicking a wire offers connection actions
    When I right-click a connection
    Then I can Remove the connection
    And Reset routing when it has a hand-routed lane

  @ui
  Scenario: Right-clicking a mission offers mission actions
    When I right-click a mission
    Then I can Start mission, Open panel, Rename, and Delete

  # ── Behavioral guarantees of the menu system ────────────────────────────────

  @ui
  Scenario: Element-specific menus win over the card catch-all
    When I right-click a slot inside a formation
    Then the slot menu opens, not the formation menu
    # Layered handlers stopPropagation so the most specific menu wins.

  @ui
  Scenario: A menu closes on outside interaction
    Given a context menu is open
    When I click elsewhere or scroll
    Then the menu closes

  @ui
  Scenario: Destructive items are marked and every mutating action is undoable
    When I choose any "Delete"/"Remove" item
    Then it is visually marked as destructive
    And the action can be reverted with undo (see canvas.feature)

  @ui
  Scenario: Menus stay on screen
    When I right-click near a screen edge
    Then the menu is repositioned to remain fully visible
