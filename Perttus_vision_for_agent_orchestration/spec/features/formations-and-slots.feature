# Captures formations + slots (03-formations.js: TYPES, makeFormation, bodyHTML per type,
# beginPointer agent drag-to-slot, assignDirect, addSlot, Make controller, dupFormation,
# briefs/input). A slot reference is NOT ownership — one agent may fill many slots/formations.

Feature: Formations and slots — four coordination shapes staffed by live agents
  As an agent (or human) assembling a unit of coordinated work
  I need formations of the right shape, with slots I can staff, add, and re-role
  So that the unit expresses how the work is divided among agents

  Background:
    Given a board "session-search"
    And a live agent roster including "conductor", "codex", "mason", "scout", "susie"

  # ── The four types ──────────────────────────────────────────────────────────

  @ui @file
  Scenario Outline: Each formation type carries its meaning and default slots
    When I create a "<type>" formation
    Then it is persisted with type "<type>" and the default slots "<slots>"
    And it renders in its characteristic layout
    Examples:
      | type         | slots                                  |
      | solo         | one agent slot                         |
      | peer         | two equal peer slots, no hierarchy     |
      | flow         | ordered, numbered steps (A → B → C)    |
      | orchestrated | one controller + a pool of agent slots |

  @cli @file
  Scenario: A formation is authored from the CLI with a title and type
    When I run "archon formation create session-search peer --title 'Research huddle'"
    Then a peer formation "Research huddle" exists with two peer slots

  @ui @file
  Scenario: Orchestrated formations have exactly one controller
    Given an orchestrated formation with a controller and three worker slots
    When I choose "Make controller" on a worker slot
    Then that slot becomes the controller
    And the previous controller becomes a worker
    And there is never more than one controller

  # ── Staffing slots ──────────────────────────────────────────────────────────

  @ui @file
  Scenario: Drag an agent from the roster and drop it into a slot
    When I drag "codex" from the roster onto an empty slot
    Then the slot snaps to the nearest target and is filled by "codex"
    And the roster marks "codex" as deployed

  @ui @file
  Scenario: Assign an agent via the slot's menu instead of dragging
    When I right-click a slot and pick "mason" from the roster submenu
    Then the slot is filled by "mason"

  @ui @file
  Scenario: Drag a filled slot's agent out to empty canvas to unassign
    Given a slot filled by "scout"
    When I drag the agent off the slot and release on empty canvas
    Then the slot becomes empty

  @ui @file
  Scenario: The same agent may staff multiple slots and formations
    When I assign "conductor" to a slot in "frame" and a slot in "triage"
    Then both slots reference "conductor"
    And neither assignment removes the other (placement is a reference, not ownership)

  @cli @file
  Scenario: Assignment from the CLI matches the UI
    When I run "archon formation assign session-search frame --slot slot_controller --agent conductor"
    Then the slot references agent "conductor"

  @cli @file
  Scenario: A slot may select a harness variant without storing a session name
    When I run "archon formation assign session-search frame --slot slot_peer_a --agent susie --harness openai-codex"
    Then the slot records agentId "susie" and harness "openai-codex"
    But it does not record a tmux session name
    And run-time dispatch resolves the session through susie's persona card

  # ── Editing structure ───────────────────────────────────────────────────────

  @ui @file
  Scenario: Add and remove slots live
    Given a peer formation with two slots
    When I add a slot
    Then it has three slots
    When I remove the added slot
    Then it has two slots again

  @ui @file
  Scenario: Flow steps are ordered and inserting keeps the order
    Given a flow formation with steps "Plan", "Execute", "Push"
    When I choose "Add step after" on "Plan"
    Then the new step is inserted between "Plan" and "Execute"
    And the steps are renumbered in order

  @ui @file
  Scenario: Duplicating a formation copies its shape and verification but not run output
    Given a formation with a verification and a completed output
    When I duplicate it
    Then the copy has the same type, slots, and verification with fresh ids
    And the copy has no output (output is produced by a run, never copied)

  # ── The brief (manual input) ────────────────────────────────────────────────

  @ui @file
  Scenario: A formation's brief carries a goal, an optional bead, and file/link refs
    When I set the input goal, attach bead "bd-204", and add a file ref "src/SessionPanel.tsx"
    Then the brief persists those fields
    And the brief input port shows it has manual input

  @cli @file
  Scenario: The brief is authorable from the CLI, including bead linkage
    When I run "archon formation set-brief session-search frame --goal 'Frame the goal' --bead bd-204"
    Then the brief records the goal and bead id
    # Bead resolution (reading the work item, writing results back) is a run-engine integration point.

  @file
  Scenario: A slot reference resolves to a live session only at run time
    Given a slot references agent "scout"
    When no live session matches "scout"
    Then the structure is still valid (the reference persists)
    But a run dispatch to that slot fails loud (see run-recovery.feature)
