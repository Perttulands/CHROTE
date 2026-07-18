# Accepted-target illustrative whole-system scenario, with executable acceptance owned by
# ctx-rul and ctx-ug7.14. It describes the intended file-backed Formations experience rather
# than claiming that the current runtime already provides it.

Feature: End-to-end — a web experience to support a job search, driven through the Archon
  As Perttu
  I want to give one goal to the Archon and have a team produce a real artifact
  So that I access many specialists/harnesses without holding the coordination myself

  Background:
    Given a workspace with persona cards and the file-backed Formations model available as an always-on capability

  @e2e
  Scenario: One goal becomes a staffed, run, recovered, and judged mission
    # One touchpoint — Perttu never names a socket or session
    Given Perttu tells the Archon: "I want a web experience that showcases my agentic-engineering work for an AI-company job search"
    When the Archon frames the goal and assembles a team
    Then it picks or creates (factory) the agents it needs — e.g. a design lead and a frontend specialist — writing any new persona cards
    And it builds a mission "showcase-site" with formations and at least one gate, backed by a bead

    When the Archon runs the mission
    Then work is dispatched cross-harness (e.g. a Claude Code design lead briefs a Codex frontend specialist) as ordinary tmux dispatch
    And a real artifact is produced in the workspace (e.g. a prototype "index.html")
    And the ledger shows the cascade: run_started → node_started → slot_dispatch → slot_result → node_output → gate_verdict

    When Perttu asks the Archon "what came back?"
    Then it answers in plain language from the ledger, and the Formations tab shows the same truth

    When Perttu disconnects mid-run and later reconnects
    Then the coordinator continues the run independently while no browser is connected
    And reconnecting restores observation of the same ledger-backed run
    And no operator run resume is implied or issued solely because the browser disconnected

    When a specialist's pane is killed mid-run
    Then the ledger records a loud error and the Archon says that node went dark

    When a gate needs Perttu's taste
    Then it escalates and requests a human verdict
    And "archon gate approve|reject <runId> <gateId>" records a human_verdict_recorded event
    And the human_verdict_recorded event contributes to the Gate aggregate
    And only the resulting aggregate gate_verdict routes the workflow

  @e2e
  Scenario: A code rollback preserves durable Formations evidence
    Given the accepted-target Formations code has produced evidence under ".formations/"
    When the operator reverts the code or deploys a prior CHROTE code version
    Then the rollback preserves the existing ".formations/" evidence without deleting or rewriting it
    And existing tmux sessions remain untouched
    And existing Beads state remains untouched
