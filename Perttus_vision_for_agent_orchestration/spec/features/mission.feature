# Captures the mission entry point (03-formations.js: makeMission, renderMissions, runMission,
# missionChain, openMissionPanel). A mission is the entry point that starts a run; it "wraps" the
# reachable chain and is bead-backed work (missions/runs keyed by Beads IDs, per DECISIONS-LOCKED).

Feature: Mission — the entry point that starts and frames a run
  As the Archon or a team leader
  I need an entry node that seeds a goal and kicks off the downstream chain
  So that one objective drives a whole formation graph, tracked as a bead

  Background:
    Given a board "session-search"

  @ui @file
  Scenario: A mission carries an objective and a single output port
    When I create a mission and set its objective
    Then the mission persists its title and objective
    And it has exactly one output port that starts the chain

  @cli @file
  Scenario: A mission is created from the CLI and backed by a bead
    When I run "archon mission create session-search --title 'Improve session search' --goal 'Make session search fuzzy and keyboard-first'"
    Then a mission exists with that objective
    And it is linked to a backing bead id (e.g. "bd-204")

  @ui @file
  Scenario: Wiring the mission output builds the chain
    When I drag the mission's output into "frame:in[0]"
    Then "frame" becomes the first step of the mission's chain

  @ui
  Scenario: The mission panel lists every step reachable from the mission
    When I open the mission panel
    Then it lists the chain in order with a status dot per step
    And the dots reflect each step's run status (idle/running/done/needs-review/blocked, gate pass/fail)
    And an empty chain prompts me to wire the mission's output to a step

  @ui @cli
  Scenario: Starting the mission runs the whole chain
    When I start the mission (panel button, mission start, or "archon mission run session-search")
    Then the run cascades from the mission objective as the seed input (see run-execution.feature)

  @ui @file
  Scenario: Rename and delete a mission
    When I rename the mission
    Then its title updates
    When I delete the mission
    Then the mission and its connections are removed (its downstream nodes remain)

  @file
  Scenario: The chain is derived from connections, not stored separately
    When connections change
    Then the mission's chain re-derives from the reachable sub-graph
    And no duplicate chain state is persisted
