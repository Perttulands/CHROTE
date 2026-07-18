# Captures the "conversational visibility first" principle (vision §7/§15; DECISIONS-LOCKED). The
# system must be EXPLAINABLE ON REQUEST (ask the agents; they read the ledger), with the tab as an
# optional inspection surface — not a dashboard you must watch.

Feature: Visibility — explainable on request, not a dashboard you must watch
  As Perttu working through the Archon
  I need to ask what's happening and get a truthful answer from the ledger
  So that I don't have to watch a cockpit to stay informed

  Background:
    Given runs and definitions are file-backed (boards, layout, NDJSON ledger)
    And both the Archon (via the ledger) and the Formations tab read the same truth

  @cli
  Scenario: Ask the Archon "what came back?" and get a plain-language answer from the ledger
    Given a run has produced outputs and verdicts
    When Perttu asks the Archon what happened
    Then it summarizes the chain, what each step produced, and where work is waiting or blocked
    And the answer is derived from the ledger, not invented

    @cli
    Scenario: The CLI answers the same questions for an agent
      When I run "archon run status run_01J9" and "archon run logs run_01J9"
      Then I get the projected status and the event history
      And these match what the Archon says

  @ui
  Scenario: The Formations tab is optional to watch, not optional run authority
    Given Formations is available as an always-on file-backed capability
    When no browser has the tab open
    Then admitted runs and durable ledgers continue unchanged
    When I open the tab
    Then it shows the mission list, node graph, and run timeline from that same truth

  @ui
  Scenario: Progressive disclosure — overview first, detail on demand
    When I view a mission
    Then I first see its chain and per-step status
    And I drill into a node to see its brief, slots, output, report, and diffs only when I ask
    And report/diff detail resolves registered ids through the latest authorized "ArtifactProjection"

  @ui
  Scenario: The tab reflects external changes live-ish
    Given an agent mutates a board via the CLI while the tab is open
    Then the tab detects the change and reloads (a "board.changed" signal), reconciling concurrent edits

  @ui @cli
  Scenario: A live run streams to whoever is watching, but watching is never required
    When a run is in progress
    Then the tab and CLI receive the same sanitized event projection over SSE/follow
    And neither receives raw ledger bytes, private paths, or revoked artifact refs
    But the run proceeds and stays fully recoverable whether or not anyone is watching
