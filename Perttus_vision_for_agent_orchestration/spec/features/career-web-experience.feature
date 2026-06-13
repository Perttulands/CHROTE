# The whole-system acceptance scenario (vision §13, master-plan §11). It exercises the full stack
# end-to-end through one touchpoint: factory → team → mission → cross-harness run → recovery →
# escalation. If this passes behind the flags, the system "works".

Feature: End-to-end — a web experience to support a job search, driven through the Archon
  As Perttu
  I want to give one goal to the Archon and have a team produce a real artifact
  So that I access many specialists/harnesses without holding the coordination myself

  Background:
    Given a workspace with persona cards and the formations system enabled behind its flags

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

      When Perttu disconnects mid-run and returns
      Then "archon run resume <runId>" (or the Archon) continues correctly with no completed step re-run

    When a specialist's pane is killed mid-run
    Then the ledger records a loud error and the Archon says that node went dark

      When a gate needs Perttu's taste
      Then it escalates / requests a human verdict, and "archon gate approve|reject <runId> <gateId>" routes it

  @e2e
  Scenario: Clean rollback after the demo
    When "chrote-formations" is turned off and "CHROTE_FORMATIONS=off" with a restart
    Then the dashboard and server behave exactly as before the system existed
    And "rm -rf .formations/" leaves the workspace, code, sessions, and beads intact
