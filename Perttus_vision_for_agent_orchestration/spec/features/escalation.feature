# Captures escalation + human decision channels. Per DECISIONS-LOCKED: stage-1 escalation is
# ledger + the Archon surfacing it conversationally (notice board and TTS interrupt are deferred).
# Human gates are the deliberate "needs Perttu's judgment" routing point.

Feature: Escalation and human judgment — surfacing the few things that need Perttu
  As a leader/Archon running work mostly autonomously
  I need to raise the meaningful exceptions and route decisions that need taste
  So that Perttu is interrupted only when it matters, and can decide when asked

  Background:
    Given a run is in progress on board "session-search"
    And escalations are recorded in the run ledger (notice board + TTS are deferred)

  @cli
  Scenario: An agent raises an escalation via a sentinel
    When an agent emits "<<<CHROTE-ESCALATE run-id=... reason='found a better direction'>>>"
    Then the ledger records an "escalation_raised" event with the reason and run id
    And the run continues unless the escalation itself blocks (agent judgment, not a gate)

  @cli @ui
  Scenario: The Archon surfaces escalations conversationally
    When Perttu asks the Archon "anything need me?"
    Then it reports open escalations from the ledger in plain language
    And the Formations tab marks the escalating node
    # No forced interrupt in stage 1 — surfaced on request and visibly, not via TTS.

  @cli
  Scenario Outline: The meaningful reasons to interrupt are first-class escalation triggers
    When an agent escalates for "<reason>"
    Then it is recorded and surfaceable
    Examples:
      | reason                          |
      | blocked work                    |
      | needs Perttu's taste/judgment   |
      | team disagreement               |
      | architectural drift             |
      | cost/risk concern               |
      | surprising opportunity          |
      | a better direction than planned |
      | work should stop and reclarify  |

  # ── Human verdicts (the decision channel) ───────────────────────────────────

  @ui @cli
  Scenario: A human gate waits for Perttu's verdict
    Given a gate whose kinds include "human"
    When the run reaches it
    Then it records "human_input_requested" and waits
    And it does not auto-pass or auto-fail

  @cli
  Scenario: Perttu records a verdict to route the gate
    When I run "archon gate approve run_01J9 gate_01J9_review --reason 'direction is right'"
    Then the ledger records "human_verdict_recorded" as pass for the exact waiting Gate attempt
    And one aggregate "gate_verdict" records all declared kind results and alone routes the pass wire
    And "archon gate reject run_01J9 gate_01J9_review --reason ..." contributes fail to that aggregate, whose verdict alone routes the fail wire

  @ui
  Scenario: The verdict can also be given from the tab
    When I approve or reject the waiting gate in the Formations tab
    Then it issues the same operation against the same run as the CLI

  @cli
  Scenario: Multi-kind gate verdicts are strict AND
    Given a gate combining "code" and "human"
    Then it passes only if both the code check and the human verdict pass
    And a human cannot override a failing code check (edit the criterion and re-run instead)

  @cli
  Scenario: Deferred channels are explicitly out of stage 1
    Then no notice board is required for escalation to work
    And no TTS/Discord interrupt is wired yet (added later only if escalations are missed while away)
